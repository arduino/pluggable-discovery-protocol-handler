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

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/arduino/dummy-discovery/version"
	"github.com/arduino/go-properties-orderedmap"
)

var initialized = false
var started = false
var syncStarted = false
var syncCloseChan chan<- bool

func main() {
	parseArgs()
	if args.showVersion {
		fmt.Printf("serial-discovery %s (build timestamp: %s)\n", version.Tag, version.Timestamp)
		return
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fullCmd, err := reader.ReadString('\n')
		if err != nil {
			output(&genericMessageJSON{
				EventType: "command_error",
				Error:     true,
				Message:   err.Error(),
			})
			os.Exit(1)
		}
		split := strings.Split(fullCmd, " ")
		cmd := strings.ToUpper(strings.TrimSpace(split[0]))

		if !initialized && cmd != "HELLO" {
			output(&genericMessageJSON{
				EventType: "command_error",
				Error:     true,
				Message:   fmt.Sprintf("First command must be HELLO, but got '%s'", cmd),
			})
			continue
		}

		switch cmd {
		case "HELLO":
			if initialized {
				output(&genericMessageJSON{
					EventType: "hello",
					Error:     true,
					Message:   "HELLO already called",
				})
			}
			re := regexp.MustCompile(`(\d+) "([^"]+)"`)
			matches := re.FindStringSubmatch(fullCmd[6:])
			if len(matches) != 3 {
				output(&genericMessageJSON{
					EventType: "hello",
					Error:     true,
					Message:   "Invalid HELLO command",
				})
				continue
			}
			_ /* userAgent */ = matches[2]
			_ /* reqProtocolVersion */, err := strconv.ParseUint(matches[1], 10, 64)
			if err != nil {
				output(&genericMessageJSON{
					EventType: "hello",
					Error:     true,
					Message:   "Invalid protocol version: " + matches[2],
				})
				continue
			}
			output(&genericMessageJSON{
				EventType:       "hello",
				ProtocolVersion: 1, // Protocol version 1 is the only supported for now...
				Message:         "OK",
			})
			initialized = true

		case "START":
			if started {
				output(&genericMessageJSON{
					EventType: "start",
					Error:     true,
					Message:   "already STARTed",
				})
				continue
			}
			if syncStarted {
				output(&genericMessageJSON{
					EventType: "start",
					Error:     true,
					Message:   "discovery already START_SYNCed, cannot START",
				})
				continue
			}
			output(&genericMessageJSON{
				EventType: "start",
				Message:   "OK",
			})
			started = true

		case "LIST":
			if !started {
				output(&genericMessageJSON{
					EventType: "list",
					Error:     true,
					Message:   "discovery not STARTed",
				})
				continue
			}
			if syncStarted { // TODO: Report in RFC that in "events mode" LIST is not allowed
				output(&genericMessageJSON{
					EventType: "list",
					Error:     true,
					Message:   "discovery already START_SYNCed, LIST not allowed",
				})
				continue
			}
			outputList()

		case "START_SYNC":
			startSync()

		case "STOP":
			if !syncStarted && !started {
				output(&genericMessageJSON{
					EventType: "stop",
					Error:     true,
					Message:   "already STOPped",
				})
				continue
			}
			if started {
				started = false
			}
			if syncStarted {
				syncCloseChan <- true
				close(syncCloseChan)
				syncCloseChan = nil
				syncStarted = false
			}
			output(&genericMessageJSON{
				EventType: "stop",
				Message:   "OK",
			})

		case "QUIT":
			output(&genericMessageJSON{
				EventType: "quit",
				Message:   "OK",
			})
			os.Exit(0)

		default:
			output(&genericMessageJSON{
				EventType: "command_error",
				Error:     true,
				Message:   fmt.Sprintf("Command %s not supported", cmd),
			})
		}
	}
}

type boardPortJSON struct {
	Address       string          `json:"address"`
	Label         string          `json:"label,omitempty"`
	Protocol      string          `json:"protocol,omitempty"`
	ProtocolLabel string          `json:"protocolLabel,omitempty"`
	Properties    *properties.Map `json:"properties,omitempty"`
}

type listOutputJSON struct {
	EventType string           `json:"eventType"`
	Ports     []*boardPortJSON `json:"ports"`
}

type syncOutputJSON struct {
	EventType string         `json:"eventType"`
	Port      *boardPortJSON `json:"port"`
}

var startSyncCount = 0

func startSync() {
	if syncStarted {
		output(&genericMessageJSON{
			EventType: "start_sync",
			Error:     true,
			Message:   "discovery already START_SYNCed",
		})
		return
	}
	if started {
		output(&genericMessageJSON{
			EventType: "start_sync",
			Error:     true,
			Message:   "discovery already STARTed, cannot START_SYNC",
		})
		return
	}

	startSyncCount++
	if startSyncCount%5 == 0 {
		output(&genericMessageJSON{
			EventType: "start_sync",
			Error:     true,
			Message:   "could not start_sync every 5 times",
		})
		return
	}

	c := make(chan bool)

	syncCloseChan = c
	syncStarted = true
	output(&genericMessageJSON{
		EventType: "start_sync",
		Message:   "OK",
	})

	// Run synchronous event emitter
	go func() {
		var closeChan <-chan bool = c

		// Ouput initial port state
		output(&syncOutputJSON{
			EventType: "add",
			Port:      CreateDummyPort(),
		})
		output(&syncOutputJSON{
			EventType: "add",
			Port:      CreateDummyPort(),
		})

		// Start sending events
		for {
			// if err != nil {
			// 	output(&genericMessageJSON{
			// 		EventType: "start_sync",
			// 		Error:     true,
			// 		Message:   fmt.Sprintf("error decoding START_SYNC event: %s", err),
			// 	})
			// 	return
			// }

			select {
			case <-closeChan:
				return
			case <-time.After(2 * time.Second):
			}

			port := CreateDummyPort()
			output(&syncOutputJSON{
				EventType: "add",
				Port:      port,
			})

			select {
			case <-closeChan:
				return
			case <-time.After(2 * time.Second):
			}

			output(&syncOutputJSON{
				EventType: "remove",
				Port: &boardPortJSON{
					Address:  port.Address,
					Protocol: port.Protocol,
				},
			})
		}
	}()
}

var listCount = 0

func outputList() {
	listCount++
	if listCount%5 == 0 {
		output(&genericMessageJSON{
			EventType: "list",
			Error:     true,
			Message:   "could not list every 5 times",
		})
		return
	}

	portJSON := CreateDummyPort()
	portsJSON := []*boardPortJSON{portJSON}
	output(&listOutputJSON{
		EventType: "list",
		Ports:     portsJSON,
	})
}

type genericMessageJSON struct {
	EventType       string `json:"eventType"`
	Message         string `json:"message"`
	Error           bool   `json:"error,omitempty"`
	ProtocolVersion int    `json:"protocolVersion,omitempty"`
}

var dummyCounter = 0

func CreateDummyPort() *boardPortJSON {
	dummyCounter++
	return &boardPortJSON{
		Address:       fmt.Sprintf("%d", dummyCounter),
		Label:         "Dummy upload port",
		Protocol:      "dummy",
		ProtocolLabel: "Dummy protocol",
		Properties: properties.NewFromHashmap(map[string]string{
			"vid": "0x2341",
			"pid": "0x0041",
			"mac": fmt.Sprintf("%d", dummyCounter*73622384782),
		}),
	}
}

func output(msg interface{}) {
	d, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		output(&genericMessageJSON{
			EventType: "command_error",
			Error:     true,
			Message:   err.Error(),
		})
	} else {
		stdoutMutex.Lock()
		fmt.Println(string(d))
		stdoutMutex.Unlock()
	}
}

var stdoutMutex sync.Mutex
