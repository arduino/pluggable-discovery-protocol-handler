//
// This file is part of pluggable-discovery-protocol-handler.
//
// Copyright 2021 ARDUINO SA (http://www.arduino.cc/)
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

// Package discovery is a library for handling the Arduino Pluggable-Discovery protocol
// (https://github.com/arduino/tooling-rfcs/blob/main/RFCs/0002-pluggable-discovery.md#pluggable-discovery-api-via-stdinstdout)
//
// The library implements the state machine and the parsing logic to communicate with a pluggable-discovery client.
// All the commands issued by the client are conveniently translated into function calls, in particular
// the Discovery interface are the only functions that must be implemented to get a fully working pluggable discovery
// using this library.
//
// A usage example is provided in the dummy-discovery package.
package discovery

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/arduino/go-properties-orderedmap"
)

// Port is a descriptor for a board port
type Port struct {
	Address       string          `json:"address"`
	AddressLabel  string          `json:"label,omitempty"`
	Protocol      string          `json:"protocol,omitempty"`
	ProtocolLabel string          `json:"protocolLabel,omitempty"`
	Properties    *properties.Map `json:"properties,omitempty"`
}

// Discovery is an interface that represents the business logic that
// a pluggable discovery must implement. The communication protocol
// is completely hidden and it's handled by a DiscoveryServer.
type Discovery interface {
	// Hello is called once at startup to provide the userAgent string
	// and the protocolVersion negotiated with the client.
	Hello(userAgent string, protocolVersion int) error

	// StartSync is called to put the discovery in event mode. When the
	// function returns the discovery must send port events ("add" or "remove")
	// using the eventCB function.
	StartSync(eventCB EventCallback, errorCB ErrorCallback) error

	// Stop stops the discovery internal subroutines. If the discovery is
	// in event mode it must stop sending events through the eventCB previously
	// set.
	Stop() error

	// Quit is called just before the server terminates. This function can be
	// used by the discovery as a last chance gracefully close resources.
	Quit()
}

// EventCallback is a callback function to call to transmit port
// metadata when the discovery is in "sync" mode and a new event
// is detected.
type EventCallback func(event string, port *Port)

// ErrorCallback is a callback function to signal unrecoverable errors to the
// client while the discovery is in event mode. Once the discovery signal an
// error it means that no more port-events will be delivered until the client
// performs a STOP+START_SYNC cycle.
type ErrorCallback func(err string)

// A Server is a pluggable discovery protocol handler,
// it must be created using the NewServer function.
type Server struct {
	impl               Discovery
	outputChan         chan *message
	userAgent          string
	reqProtocolVersion int
	initialized        bool
	started            bool
	syncStarted        bool
	cachedPorts        map[string]*Port
	cachedErr          string
}

// NewServer creates a new discovery server backed by the
// provided pluggable discovery implementation. To start the server
// use the Run method.
func NewServer(impl Discovery) *Server {
	return &Server{
		impl:       impl,
		outputChan: make(chan *message),
	}
}

// Run starts the protocol handling loop on the given input and
// output stream, usually `os.Stdin` and `os.Stdout` are used.
// The function blocks until the `QUIT` command is received or
// the input stream is closed. In case of IO error the error is
// returned.
func (d *Server) Run(in io.Reader, out io.Writer) error {
	go d.outputProcessor(out)
	defer close(d.outputChan)
	reader := bufio.NewReader(in)
	for {
		fullCmd, err := reader.ReadString('\n')
		if err != nil {
			d.outputChan <- messageError("command_error", err.Error())
			return err
		}
		fullCmd = strings.TrimSpace(fullCmd)
		split := strings.Split(fullCmd, " ")
		cmd := strings.ToUpper(split[0])

		if !d.initialized && cmd != "HELLO" && cmd != "QUIT" {
			d.outputChan <- messageError("command_error", fmt.Sprintf("First command must be HELLO, but got '%s'", cmd))
			continue
		}

		switch cmd {
		case "HELLO":
			d.hello(fullCmd[6:])
		case "START":
			d.start()
		case "LIST":
			d.list()
		case "START_SYNC":
			d.startSync()
		case "STOP":
			d.stop()
		case "QUIT":
			d.impl.Quit()
			d.outputChan <- messageOk("quit")
			return nil
		default:
			d.outputChan <- messageError("command_error", fmt.Sprintf("Command %s not supported", cmd))
		}
	}
}

func (d *Server) hello(cmd string) {
	if d.initialized {
		d.outputChan <- messageError("hello", "HELLO already called")
		return
	}
	re := regexp.MustCompile(`^(\d+) "([^"]+)"$`)
	matches := re.FindStringSubmatch(cmd)
	if len(matches) != 3 {
		d.outputChan <- messageError("hello", "Invalid HELLO command")
		return
	}
	d.userAgent = matches[2]
	v, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		d.outputChan <- messageError("hello", "Invalid protocol version: "+matches[2])
		return
	}
	d.reqProtocolVersion = int(v)
	if err := d.impl.Hello(d.userAgent, 1); err != nil {
		d.outputChan <- messageError("hello", err.Error())
		return
	}
	d.outputChan <- &message{
		EventType:       "hello",
		ProtocolVersion: 1, // Protocol version 1 is the only supported for now...
		Message:         "OK",
	}
	d.initialized = true
}

func (d *Server) start() {
	if d.started {
		d.outputChan <- messageError("start", "Discovery already STARTed")
		return
	}
	if d.syncStarted {
		d.outputChan <- messageError("start", "Discovery already START_SYNCed, cannot START")
		return
	}
	d.cachedPorts = map[string]*Port{}
	d.cachedErr = ""
	if err := d.impl.StartSync(d.eventCallback, d.errorCallback); err != nil {
		d.outputChan <- messageError("start", "Cannot START: "+err.Error())
		return
	}
	d.started = true
	d.outputChan <- messageOk("start")
}

func (d *Server) eventCallback(event string, port *Port) {
	id := port.Address + "|" + port.Protocol
	if event == "add" {
		d.cachedPorts[id] = port
	}
	if event == "remove" {
		delete(d.cachedPorts, id)
	}
}

func (d *Server) errorCallback(msg string) {
	d.cachedErr = msg
}

func (d *Server) list() {
	if !d.started {
		d.outputChan <- messageError("list", "Discovery not STARTed")
		return
	}
	if d.syncStarted {
		d.outputChan <- messageError("list", "discovery already START_SYNCed, LIST not allowed")
		return
	}
	if d.cachedErr != "" {
		d.outputChan <- messageError("list", d.cachedErr)
		return
	}
	ports := []*Port{}
	for _, port := range d.cachedPorts {
		ports = append(ports, port)
	}
	d.outputChan <- &message{
		EventType: "list",
		Ports:     &ports,
	}
}

func (d *Server) startSync() {
	if d.syncStarted {
		d.outputChan <- messageError("start_sync", "Discovery already START_SYNCed")
		return
	}
	if d.started {
		d.outputChan <- messageError("start_sync", "Discovery already STARTed, cannot START_SYNC")
		return
	}
	if err := d.impl.StartSync(d.syncEvent, d.errorEvent); err != nil {
		d.outputChan <- messageError("start_sync", "Cannot START_SYNC: "+err.Error())
		return
	}
	d.syncStarted = true
	d.outputChan <- messageOk("start_sync")
}

func (d *Server) stop() {
	if !d.syncStarted && !d.started {
		d.outputChan <- messageError("stop", "Discovery already STOPped")
		return
	}
	if err := d.impl.Stop(); err != nil {
		d.outputChan <- messageError("stop", "Cannot STOP: "+err.Error())
		return
	}
	d.started = false
	if d.syncStarted {
		d.syncStarted = false
	}
	d.outputChan <- messageOk("stop")
}

func (d *Server) syncEvent(event string, port *Port) {
	d.outputChan <- &message{
		EventType: event,
		Port:      port,
	}
}

func (d *Server) errorEvent(msg string) {
	d.outputChan <- messageError("start_sync", msg)
}

func (d *Server) outputProcessor(outWriter io.Writer) {
	// Start go routine to serialize messages printing
	go func() {
		for msg := range d.outputChan {
			data, err := json.MarshalIndent(msg, "", "  ")
			if err != nil {
				// We are certain that this will be marshalled correctly
				// so we don't handle the error
				data, _ = json.MarshalIndent(messageError("command_error", err.Error()), "", "  ")
			}
			fmt.Fprintln(outWriter, string(data))
		}
	}()
}
