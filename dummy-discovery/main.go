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
	"errors"
	"fmt"
	"os"
	"time"

	discovery "github.com/arduino/dummy-discovery"
	"github.com/arduino/dummy-discovery/dummy-discovery/args"
	"github.com/arduino/go-properties-orderedmap"
)

type DummyDiscovery struct {
	startSyncCount int
	listCount      int
}

func main() {
	args.ParseArgs()
	dummyDiscovery := &DummyDiscovery{}
	server := discovery.NewDiscoveryServer(dummyDiscovery)
	if err := server.Run(os.Stdin, os.Stdout); err != nil {
		os.Exit(1)
	}
}

func (d *DummyDiscovery) Hello(userAgent string, protocol int) error {
	return nil
}

func (d *DummyDiscovery) List() ([]*discovery.Port, error) {
	d.listCount++
	if d.listCount%5 == 0 {
		return nil, errors.New("could not list every 5 times")
	}
	return []*discovery.Port{
		CreateDummyPort(),
	}, nil
}

func (d *DummyDiscovery) Start() error {
	return nil
}

func (d *DummyDiscovery) Stop() error {
	return nil
}

func (d *DummyDiscovery) StartSync(eventCB discovery.EventCallback) (chan<- bool, error) {
	d.startSyncCount++
	if d.startSyncCount%5 == 0 {
		return nil, errors.New("could not start_sync every 5 times")
	}

	c := make(chan bool)

	// Run synchronous event emitter
	go func() {
		var closeChan <-chan bool = c

		// Ouput initial port state
		eventCB("add", CreateDummyPort())
		eventCB("add", CreateDummyPort())

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
			eventCB("add", port)

			select {
			case <-closeChan:
				return
			case <-time.After(2 * time.Second):
			}

			eventCB("remove", &discovery.Port{
				Address:  port.Address,
				Protocol: port.Protocol,
			})
		}
	}()

	return c, nil
}

var dummyCounter = 0

func CreateDummyPort() *discovery.Port {
	dummyCounter++
	return &discovery.Port{
		Address:       fmt.Sprintf("%d", dummyCounter),
		AddressLabel:  "Dummy upload port",
		Protocol:      "dummy",
		ProtocolLabel: "Dummy protocol",
		Properties: properties.NewFromHashmap(map[string]string{
			"vid": "0x2341",
			"pid": "0x0041",
			"mac": fmt.Sprintf("%d", dummyCounter*384782),
		}),
	}
}
