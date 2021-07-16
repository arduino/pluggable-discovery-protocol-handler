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

type Port struct {
	Address       string          `json:"address"`
	AddressLabel  string          `json:"label,omitempty"`
	Protocol      string          `json:"protocol,omitempty"`
	ProtocolLabel string          `json:"protocolLabel,omitempty"`
	Properties    *properties.Map `json:"properties,omitempty"`
}

type EventCallback func(event string, port *Port)

type Discovery interface {
	Hello(userAgent string, protocolVersion int) error
	Start() error
	Stop() error
	List() ([]*Port, error)
	StartSync(eventCB EventCallback) (chan<- bool, error)
}

type DiscoveryServer struct {
	impl               Discovery
	out                io.Writer
	outMutex           sync.Mutex
	userAgent          string
	reqProtocolVersion int
	initialized        bool
	started            bool
	syncStarted        bool
	syncCloseChan      chan<- bool
}

func NewDiscoveryServer(impl Discovery) *DiscoveryServer {
	return &DiscoveryServer{
		impl: impl,
	}
}

func (d *DiscoveryServer) Run(in io.Reader, out io.Writer) error {
	d.out = out
	reader := bufio.NewReader(in)
	for {
		fullCmd, err := reader.ReadString('\n')
		if err != nil {
			d.output(&genericMessageJSON{
				EventType: "command_error",
				Error:     true,
				Message:   err.Error(),
			})
			return err
		}
		split := strings.Split(fullCmd, " ")
		cmd := strings.ToUpper(strings.TrimSpace(split[0]))

		if !d.initialized && cmd != "HELLO" {
			d.output(&genericMessageJSON{
				EventType: "command_error",
				Error:     true,
				Message:   fmt.Sprintf("First command must be HELLO, but got '%s'", cmd),
			})
			continue
		}

		switch cmd {
		case "HELLO":
			if d.initialized {
				d.output(&genericMessageJSON{
					EventType: "hello",
					Error:     true,
					Message:   "HELLO already called",
				})
				continue
			}
			re := regexp.MustCompile(`(\d+) "([^"]+)"`)
			matches := re.FindStringSubmatch(fullCmd[6:])
			if len(matches) != 3 {
				d.output(&genericMessageJSON{
					EventType: "hello",
					Error:     true,
					Message:   "Invalid HELLO command",
				})
				continue
			}
			d.userAgent = matches[2]
			if v, err := strconv.ParseInt(matches[1], 10, 64); err != nil {
				d.output(&genericMessageJSON{
					EventType: "hello",
					Error:     true,
					Message:   "Invalid protocol version: " + matches[2],
				})
				continue
			} else {
				d.reqProtocolVersion = int(v)
			}
			if err := d.impl.Hello(d.userAgent, 1); err != nil {
				d.output(&genericMessageJSON{
					EventType: "hello",
					Error:     true,
					Message:   err.Error(),
				})
				continue
			}
			d.output(&genericMessageJSON{
				EventType:       "hello",
				ProtocolVersion: 1, // Protocol version 1 is the only supported for now...
				Message:         "OK",
			})
			d.initialized = true

		case "START":
			if d.started {
				d.output(&genericMessageJSON{
					EventType: "start",
					Error:     true,
					Message:   "Discovery already STARTed",
				})
				continue
			}
			if d.syncStarted {
				d.output(&genericMessageJSON{
					EventType: "start",
					Error:     true,
					Message:   "Discovery already START_SYNCed, cannot START",
				})
				continue
			}
			if err := d.impl.Start(); err != nil {
				d.output(&genericMessageJSON{
					EventType: "start",
					Error:     true,
					Message:   "Cannot START: " + err.Error(),
				})
				continue
			}
			d.started = true
			d.output(&genericMessageJSON{
				EventType: "start",
				Message:   "OK",
			})

		case "LIST":
			if !d.started {
				d.output(&genericMessageJSON{
					EventType: "list",
					Error:     true,
					Message:   "Discovery not STARTed",
				})
				continue
			}
			if d.syncStarted {
				d.output(&genericMessageJSON{
					EventType: "list",
					Error:     true,
					Message:   "discovery already START_SYNCed, LIST not allowed",
				})
				continue
			}
			if ports, err := d.impl.List(); err != nil {
				d.output(&genericMessageJSON{
					EventType: "list",
					Error:     true,
					Message:   "LIST error: " + err.Error(),
				})
				continue
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

		case "START_SYNC":
			if d.syncStarted {
				d.output(&genericMessageJSON{
					EventType: "start_sync",
					Error:     true,
					Message:   "Discovery already START_SYNCed",
				})
				continue
			}
			if d.started {
				d.output(&genericMessageJSON{
					EventType: "start_sync",
					Error:     true,
					Message:   "Discovery already STARTed, cannot START_SYNC",
				})
				continue
			}
			if c, err := d.impl.StartSync(d.syncEvent); err != nil {
				d.output(&genericMessageJSON{
					EventType: "start_sync",
					Error:     true,
					Message:   "Cannot START_SYNC: " + err.Error(),
				})
				continue
			} else {
				d.syncCloseChan = c
				d.syncStarted = true
				d.output(&genericMessageJSON{
					EventType: "start_sync",
					Message:   "OK",
				})
			}

		case "STOP":
			if !d.syncStarted && !d.started {
				d.output(&genericMessageJSON{
					EventType: "stop",
					Error:     true,
					Message:   "Discovery already STOPped",
				})
				continue
			}
			if err := d.impl.Stop(); err != nil {
				d.output(&genericMessageJSON{
					EventType: "stop",
					Error:     true,
					Message:   "Cannot STOP: " + err.Error(),
				})
				continue
			}
			if d.started {
				d.started = false
			}
			if d.syncStarted {
				d.syncCloseChan <- true
				close(d.syncCloseChan)
				d.syncStarted = false
			}
			d.output(&genericMessageJSON{
				EventType: "stop",
				Message:   "OK",
			})

		case "QUIT":
			d.output(&genericMessageJSON{
				EventType: "quit",
				Message:   "OK",
			})
			return nil

		default:
			d.output(&genericMessageJSON{
				EventType: "command_error",
				Error:     true,
				Message:   fmt.Sprintf("Command %s not supported", cmd),
			})
		}
	}
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
		fmt.Println(string(data))
		d.outMutex.Unlock()
	}
}
