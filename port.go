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

import "github.com/arduino/go-properties-orderedmap"

// Port is a descriptor for a board port
type Port struct {
	Address       string          `json:"address"`
	AddressLabel  string          `json:"label,omitempty"`
	Protocol      string          `json:"protocol,omitempty"`
	ProtocolLabel string          `json:"protocolLabel,omitempty"`
	Properties    *properties.Map `json:"properties,omitempty"`
	HardwareID    string          `json:"hardwareId,omitempty"`
}

// Equals returns true if the given port has the same address and protocol
// of the current port.
func (p *Port) Equals(o *Port) bool {
	return p.Address == o.Address && p.Protocol == o.Protocol
}

func (p *Port) String() string {
	if p == nil {
		return "none"
	}
	return p.Address
}

// Clone creates a copy of this Port
func (p *Port) Clone() *Port {
	if p == nil {
		return nil
	}
	res := *p
	if p.Properties != nil {
		res.Properties = p.Properties.Clone()
	}
	return &res
}
