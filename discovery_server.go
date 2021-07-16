//
// This file is part of dummy-discovery.
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

// discovery is a library for handling the Arduino Pluggable-Discovery protocol
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
	"sync"

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

	// Start is called to start the discovery internal subroutines.
	Start() error

	// List returns the list of the currently available ports. It works
	// only after a Start.
	List() (portList []*Port, err error)

	// StartSync is called to put the discovery in event mode. When the
	// function returns the discovery must send port events ("add" or "remove")
	// using the eventCB function.
	StartSync(eventCB EventCallback) error

	// Stop stops the discovery internal subroutines. If the discovery is
	// in event mode it must stop sending events through the eventCB previously
	// set.
	Stop() error
}

// EventCallback is a callback function to call to transmit port
// metadata when the discovery is in "sync" mode and a new event
// is detected.
type EventCallback func(event string, port *Port)

// A DiscoveryServer is a pluggable discovery protocol handler,
// it must be created using the NewDiscoveryServer function.
type DiscoveryServer struct {
	impl               Discovery
	out                io.Writer
	outMutex           sync.Mutex
	userAgent          string
	reqProtocolVersion int
	initialized        bool
	started            bool
	syncStarted        bool
}

// NewDiscoveryServer creates a new discovery server backed by the
// provided pluggable discovery implementation. To start the server
// use the Run method.
func NewDiscoveryServer(impl Discovery) *DiscoveryServer {
	return &DiscoveryServer{
		impl: impl,
	}
}

// Run starts the protocol handling loop on the given input and
// output stream, usually `os.Stdin` and `os.Stdout` are used.
// The function blocks until the `QUIT` command is received or
// the input stream is closed. In case of IO error the error is
// returned.
func (d *DiscoveryServer) Run(in io.Reader, out io.Writer) error {
	d.out = out
	reader := bufio.NewReader(in)
	for {
		fullCmd, err := reader.ReadString('\n')
		if err != nil {
			d.outputError("command_error", err.Error())
			return err
		}
		split := strings.Split(fullCmd, " ")
		cmd := strings.ToUpper(strings.TrimSpace(split[0]))

		if !d.initialized && cmd != "HELLO" {
			d.outputError("command_error", fmt.Sprintf("First command must be HELLO, but got '%s'", cmd))
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
			d.outputOk("quit")
			return nil
		default:
			d.outputError("command_error", fmt.Sprintf("Command %s not supported", cmd))
		}
	}
}

func (d *DiscoveryServer) hello(cmd string) {
	if d.initialized {
		d.outputError("hello", "HELLO already called")
		return
	}
	re := regexp.MustCompile(`(\d+) "([^"]+)"`)
	matches := re.FindStringSubmatch(cmd)
	if len(matches) != 3 {
		d.outputError("hello", "Invalid HELLO command")
		return
	}
	d.userAgent = matches[2]
	if v, err := strconv.ParseInt(matches[1], 10, 64); err != nil {
		d.outputError("hello", "Invalid protocol version: "+matches[2])
		return
	} else {
		d.reqProtocolVersion = int(v)
	}
	if err := d.impl.Hello(d.userAgent, 1); err != nil {
		d.outputError("hello", err.Error())
		return
	}
	d.output(&genericMessageJSON{
		EventType:       "hello",
		ProtocolVersion: 1, // Protocol version 1 is the only supported for now...
		Message:         "OK",
	})
	d.initialized = true
}

func (d *DiscoveryServer) start() {
	if d.started {
		d.outputError("start", "Discovery already STARTed")
		return
	}
	if d.syncStarted {
		d.outputError("start", "Discovery already START_SYNCed, cannot START")
		return
	}
	if err := d.impl.Start(); err != nil {
		d.outputError("start", "Cannot START: "+err.Error())
		return
	}
	d.started = true
	d.outputOk("start")
}

func (d *DiscoveryServer) list() {
	if !d.started {
		d.outputError("list", "Discovery not STARTed")
		return
	}
	if d.syncStarted {
		d.outputError("list", "discovery already START_SYNCed, LIST not allowed")
		return
	}
	if ports, err := d.impl.List(); err != nil {
		d.outputError("list", "LIST error: "+err.Error())
		return
	} else {
		type listOutputJSON struct {
			EventType string  `json:"eventType"`
			Ports     []*Port `json:"ports"`
		}
		d.output(&listOutputJSON{
			EventType: "list",
			Ports:     ports,
		})
	}
}

func (d *DiscoveryServer) startSync() {
	if d.syncStarted {
		d.outputError("start_sync", "Discovery already START_SYNCed")
		return
	}
	if d.started {
		d.outputError("start_sync", "Discovery already STARTed, cannot START_SYNC")
		return
	}
	if err := d.impl.StartSync(d.syncEvent); err != nil {
		d.outputError("start_sync", "Cannot START_SYNC: "+err.Error())
		return
	}
	d.syncStarted = true
	d.outputOk("start_sync")
}

func (d *DiscoveryServer) stop() {
	if !d.syncStarted && !d.started {
		d.outputError("stop", "Discovery already STOPped")
		return
	}
	if err := d.impl.Stop(); err != nil {
		d.outputError("stop", "Cannot STOP: "+err.Error())
		return
	}
	d.started = false
	d.syncStarted = false
	d.outputOk("stop")
}

func (d *DiscoveryServer) syncEvent(event string, port *Port) {
	type syncOutputJSON struct {
		EventType string `json:"eventType"`
		Port      *Port  `json:"port"`
	}
	d.output(&syncOutputJSON{
		EventType: event,
		Port:      port,
	})
}

type genericMessageJSON struct {
	EventType       string `json:"eventType"`
	Message         string `json:"message"`
	Error           bool   `json:"error,omitempty"`
	ProtocolVersion int    `json:"protocolVersion,omitempty"`
}

func (d *DiscoveryServer) outputOk(event string) {
	d.output(&genericMessageJSON{
		EventType: event,
		Message:   "OK",
	})
}

func (d *DiscoveryServer) outputError(event, msg string) {
	d.output(&genericMessageJSON{
		EventType: event,
		Error:     true,
		Message:   msg,
	})
}

func (d *DiscoveryServer) output(msg interface{}) {
	data, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		d.output(&genericMessageJSON{
			EventType: "command_error",
			Error:     true,
			Message:   err.Error(),
		})
	} else {
		d.outMutex.Lock()
		d.out.Write(data)
		d.outMutex.Unlock()
	}
}
