// This file is part of pluggable-discovery-protocol-handler.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

// Proxy stdin/stdout through a TCP socket.
// This program is used for testing purposes, to make it available on all
// OS a tool equivalent to UNIX "nc".
package main

import (
	"io"
	"net"
	"os"
)

func main() {
	tcpAddr, err := net.ResolveTCPAddr("tcp", os.Args[1])
	if err != nil {
		println("ResolveTCPAddr failed:", err.Error())
		os.Exit(1)
	}

	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		println("Dial failed:", err.Error())
		os.Exit(1)
	}

	go func() {
		io.Copy(os.Stdout, conn)
		os.Exit(0)
	}()
	io.Copy(conn, os.Stdin)
	os.Exit(0)
}
