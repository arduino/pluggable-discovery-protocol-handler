// This file is part of pluggable-discovery-protocol-handler.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

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
