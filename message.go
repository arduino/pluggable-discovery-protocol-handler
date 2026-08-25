// This file is part of pluggable-discovery-protocol-handler.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

type message struct {
	EventType       string   `json:"eventType"`
	Message         string   `json:"message,omitempty"`
	Error           bool     `json:"error,omitempty"`
	ProtocolVersion int      `json:"protocolVersion,omitempty"`
	Port            *Port    `json:"port,omitempty"`
	Ports           *[]*Port `json:"ports,omitempty"`
}

func messageOk(event string) *message {
	return &message{
		EventType: event,
		Message:   "OK",
	}
}

func messageError(event, msg string) *message {
	return &message{
		EventType: event,
		Error:     true,
		Message:   msg,
	}
}
