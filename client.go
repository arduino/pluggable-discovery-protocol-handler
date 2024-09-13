//
// This file is part of pluggable-discovery-protocol-handler.
//
// Copyright 2024 ARDUINO SA (http://www.arduino.cc/)
//
// This software is released under the GNU General Public License version 3,
// which covers the main part of arduino-cli.
// The terms of this license can be found at:
// https://www.gnu.org/licenses/gpl-3.0.en.html
//
// You can be released from the requirements of the above licenses by purchasing
// a commercial license. Buying such a license is mandatory if you want to modify or
// otherwise use the software for commercial activities involving the Arduino
// software without disclosing the source code of your own applications. To purchase
// a commercial license, send an email to license@arduino.cc.
//

package discovery

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/arduino/go-paths-helper"
)

// Client is a tool that detects communication ports to interact
// with the boards.
type Client struct {
	id                   string
	processArgs          []string
	process              *paths.Process
	outgoingCommandsPipe io.Writer
	incomingMessagesChan <-chan *discoveryMessage
	userAgent            string
	logger               ClientLogger

	// All the following fields are guarded by statusMutex
	statusMutex           sync.Mutex
	incomingMessagesError error
	eventChan             chan<- *Event
}

// ClientLogger is the interface that must be implemented by a logger
// to be used in the discovery client.
type ClientLogger interface {
	Debugf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

type nullClientLogger struct{}

func (l *nullClientLogger) Debugf(format string, args ...interface{}) {}
func (l *nullClientLogger) Errorf(format string, args ...interface{}) {}

type discoveryMessage struct {
	EventType       string  `json:"eventType"`
	Message         string  `json:"message"`
	Error           bool    `json:"error"`
	ProtocolVersion int     `json:"protocolVersion"` // Used in HELLO command
	Ports           []*Port `json:"ports"`           // Used in LIST command
	Port            *Port   `json:"port"`            // Used in add and remove events
}

func (msg discoveryMessage) String() string {
	s := fmt.Sprintf("type: %s", msg.EventType)
	if msg.Message != "" {
		s += fmt.Sprintf(", message: %s", msg.Message)
	}
	if msg.ProtocolVersion != 0 {
		s += fmt.Sprintf(", protocol version: %d", msg.ProtocolVersion)
	}
	if len(msg.Ports) > 0 {
		s += fmt.Sprintf(", ports: %s", msg.Ports)
	}
	if msg.Port != nil {
		s += fmt.Sprintf(", port: %s", msg.Port)
	}
	return s
}

// Event is a pluggable discovery event
type Event struct {
	Type        string
	Port        *Port
	DiscoveryID string
}

// NewClient create a new pluggable discovery client
func NewClient(id string, args ...string) *Client {
	return &Client{
		id:          id,
		processArgs: args,
		userAgent:   "pluggable-discovery-protocol-handler",
		logger:      &nullClientLogger{},
	}
}

// SetUserAgent sets the user agent to be used in the discovery
func (disc *Client) SetUserAgent(userAgent string) {
	disc.userAgent = userAgent
}

// SetLogger sets the logger to be used in the discovery
func (disc *Client) SetLogger(logger ClientLogger) {
	disc.logger = logger
}

// GetID returns the identifier for this discovery
func (disc *Client) GetID() string {
	return disc.id
}

func (disc *Client) String() string {
	return disc.id
}

func (disc *Client) jsonDecodeLoop(in io.Reader, outChan chan<- *discoveryMessage) {
	decoder := json.NewDecoder(in)
	closeAndReportError := func(err error) {
		disc.statusMutex.Lock()
		disc.incomingMessagesError = err
		disc.statusMutex.Unlock()
		disc.stopSync()
		disc.killProcess()
		close(outChan)
		if err != nil {
			disc.logger.Errorf("Stopped decode loop: %v", err)
		} else {
			disc.logger.Debugf("Stopped decode loop")
		}
	}

	for {
		var msg discoveryMessage
		if err := decoder.Decode(&msg); errors.Is(err, io.EOF) {
			// This is fine :flames: we exit gracefully
			closeAndReportError(nil)
			return
		} else if err != nil {
			closeAndReportError(err)
			return
		}
		disc.logger.Debugf("Received message %s", msg)
		if msg.EventType == "add" {
			if msg.Port == nil {
				closeAndReportError(errors.New("invalid 'add' message: missing port"))
				return
			}
			disc.statusMutex.Lock()
			if disc.eventChan != nil {
				disc.eventChan <- &Event{"add", msg.Port, disc.GetID()}
			}
			disc.statusMutex.Unlock()
		} else if msg.EventType == "remove" {
			if msg.Port == nil {
				closeAndReportError(errors.New("invalid 'remove' message: missing port"))
				return
			}
			disc.statusMutex.Lock()
			if disc.eventChan != nil {
				disc.eventChan <- &Event{"remove", msg.Port, disc.GetID()}
			}
			disc.statusMutex.Unlock()
		} else {
			outChan <- &msg
		}
	}
}

// Alive returns true if the discovery is running and false otherwise.
func (disc *Client) Alive() bool {
	disc.statusMutex.Lock()
	defer disc.statusMutex.Unlock()
	return disc.process != nil
}

func (disc *Client) waitMessage(timeout time.Duration) (*discoveryMessage, error) {
	select {
	case msg := <-disc.incomingMessagesChan:
		if msg == nil {
			return nil, disc.incomingMessagesError
		}
		return msg, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for message from %s", disc)
	}
}

func (disc *Client) sendCommand(command string) error {
	disc.logger.Debugf("Sending command %s", strings.TrimSpace(command))
	data := []byte(command)
	for {
		n, err := disc.outgoingCommandsPipe.Write(data)
		if err != nil {
			return err
		}
		if n == len(data) {
			return nil
		}
		data = data[n:]
	}
}

func (disc *Client) runProcess() error {
	disc.logger.Debugf("Starting discovery process")
	proc, err := paths.NewProcess(nil, disc.processArgs...)
	if err != nil {
		return err
	}
	stdout, err := proc.StdoutPipe()
	if err != nil {
		return err
	}
	stdin, err := proc.StdinPipe()
	if err != nil {
		return err
	}
	disc.outgoingCommandsPipe = stdin

	messageChan := make(chan *discoveryMessage)
	disc.incomingMessagesChan = messageChan
	go disc.jsonDecodeLoop(stdout, messageChan)

	if err := proc.Start(); err != nil {
		return err
	}

	disc.statusMutex.Lock()
	defer disc.statusMutex.Unlock()
	disc.process = proc
	disc.logger.Debugf("Discovery process started")
	return nil
}

func (disc *Client) killProcess() {
	disc.statusMutex.Lock()
	defer disc.statusMutex.Unlock()

	disc.logger.Debugf("Killing discovery process")
	if process := disc.process; process != nil {
		disc.process = nil
		if err := process.Kill(); err != nil {
			disc.logger.Errorf("Killing discovery process: %v", err)
		}
		if err := process.Wait(); err != nil {
			disc.logger.Errorf("Waiting discovery process termination: %v", err)
		}
	}
	disc.logger.Debugf("Discovery process killed")
}

// Run starts the discovery executable process and sends the HELLO command to the discovery to agree on the
// pluggable discovery protocol. This must be the first command to run in the communication with the discovery.
// If the process is started but the HELLO command fails the process is killed.
func (disc *Client) Run() (err error) {
	if err = disc.runProcess(); err != nil {
		return err
	}

	defer func() {
		// If the discovery process is started successfully but the HELLO handshake
		// fails the discovery is an unusable state, we kill the process to avoid
		// further issues down the line.
		if err == nil {
			return
		}
		disc.killProcess()
	}()

	if err = disc.sendCommand("HELLO 1 \"arduino-cli " + disc.userAgent + "\"\n"); err != nil {
		return err
	}
	if msg, err := disc.waitMessage(time.Second * 10); err != nil {
		return fmt.Errorf("calling HELLO: %w", err)
	} else if msg.EventType != "hello" {
		return fmt.Errorf("event out of sync, expected 'hello', received '%s'", msg.EventType)
	} else if msg.Error {
		return fmt.Errorf("command failed: %s", msg.Message)
	} else if strings.ToUpper(msg.Message) != "OK" {
		return fmt.Errorf("communication out of sync, expected 'OK', received '%s'", msg.Message)
	} else if msg.ProtocolVersion > 1 {
		return fmt.Errorf("protocol version not supported: requested 1, got %d", msg.ProtocolVersion)
	}
	return nil
}

// Start initializes and start the discovery internal subroutines. This command must be
// called before List.
func (disc *Client) Start() error {
	if err := disc.sendCommand("START\n"); err != nil {
		return err
	}
	if msg, err := disc.waitMessage(time.Second * 10); err != nil {
		return fmt.Errorf("calling START: %w", err)
	} else if msg.EventType != "start" {
		return fmt.Errorf("event out of sync, expected 'start', received '%s'", msg.EventType)
	} else if msg.Error {
		return fmt.Errorf("command failed: %s", msg.Message)
	} else if strings.ToUpper(msg.Message) != "OK" {
		return fmt.Errorf("communication out of sync, expected 'OK', received '%s'", msg.Message)
	}
	return nil
}

// Stop stops the discovery internal subroutines and possibly free the internally
// used resources. This command should be called if the client wants to pause the
// discovery for a while.
func (disc *Client) Stop() error {
	if err := disc.sendCommand("STOP\n"); err != nil {
		return err
	}
	if msg, err := disc.waitMessage(time.Second * 10); err != nil {
		return fmt.Errorf("calling STOP: %w", err)
	} else if msg.EventType != "stop" {
		return fmt.Errorf("event out of sync, expected 'stop', received '%s'", msg.EventType)
	} else if msg.Error {
		return fmt.Errorf("command failed: %s", msg.Message)
	} else if strings.ToUpper(msg.Message) != "OK" {
		return fmt.Errorf("communication out of sync, expected 'OK', received '%s'", msg.Message)
	}
	disc.statusMutex.Lock()
	defer disc.statusMutex.Unlock()
	disc.stopSync()
	return nil
}

func (disc *Client) stopSync() {
	if disc.eventChan != nil {
		disc.eventChan <- &Event{"stop", nil, disc.GetID()}
		close(disc.eventChan)
		disc.eventChan = nil
	}
}

// Quit terminates the discovery. No more commands can be accepted by the discovery.
func (disc *Client) Quit() {
	_ = disc.sendCommand("QUIT\n")
	if _, err := disc.waitMessage(time.Second * 5); err != nil {
		disc.logger.Errorf("Quitting discovery: %s", err)
	}
	disc.stopSync()
	disc.killProcess()
}

// List executes an enumeration of the ports and returns a list of the available
// ports at the moment of the call.
func (disc *Client) List() ([]*Port, error) {
	if err := disc.sendCommand("LIST\n"); err != nil {
		return nil, err
	}
	if msg, err := disc.waitMessage(time.Second * 10); err != nil {
		return nil, fmt.Errorf("calling LIST: %w", err)
	} else if msg.EventType != "list" {
		return nil, fmt.Errorf("event out of sync, expected 'list', received '%s'", msg.EventType)
	} else if msg.Error {
		return nil, fmt.Errorf("command failed: %s", msg.Message)
	} else {
		return msg.Ports, nil
	}
}

// StartSync puts the discovery in "events" mode: the discovery will send "add"
// and "remove" events each time a new port is detected or removed respectively.
// After calling StartSync an initial burst of "add" events may be generated to
// report all the ports available at the moment of the start.
// It also creates a channel used to receive events from the pluggable discovery.
// The event channel must be consumed as quickly as possible since it may block the
// discovery if it becomes full. The channel size is configurable.
func (disc *Client) StartSync(size int) (<-chan *Event, error) {
	disc.statusMutex.Lock()
	defer disc.statusMutex.Unlock()

	if err := disc.sendCommand("START_SYNC\n"); err != nil {
		return nil, err
	}

	if msg, err := disc.waitMessage(time.Second * 10); err != nil {
		return nil, fmt.Errorf("calling START_SYNC: %w", err)
	} else if msg.EventType != "start_sync" {
		return nil, fmt.Errorf("evemt out of sync, expected 'start_sync', received '%s'", msg.EventType)
	} else if msg.Error {
		return nil, fmt.Errorf("command failed: %s", msg.Message)
	} else if strings.ToUpper(msg.Message) != "OK" {
		return nil, fmt.Errorf("communication out of sync, expected 'OK', received '%s'", msg.Message)
	}

	// In case there is already an existing event channel in use we close it before creating a new one.
	disc.stopSync()
	c := make(chan *Event, size)
	disc.eventChan = c
	return c, nil
}
