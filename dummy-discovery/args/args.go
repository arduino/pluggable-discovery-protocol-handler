// This file is part of dummy-discovery.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

// Package args contains arguments parsing subroutines for the dummy-discovery binary.
package args

import (
	"fmt"
	"os"
	"time"
)

// Tag is the current git tag
var Tag = "snapshot"

// Timestamp is the current timestamp
var Timestamp = "unknown"

// Parse arguments passed by the user
func Parse() {
	for _, arg := range os.Args[1:] {
		if arg == "" {
			continue
		}
		if arg == "-v" || arg == "--version" {
			fmt.Printf("dummy-discovery %s (build timestamp: %s)\n", Tag, Timestamp)
			os.Exit(0)
		}
		if arg == "-k" {
			// Emulate crashing discovery
			go func() {
				time.Sleep(time.Millisecond * 500)
				os.Exit(1)
			}()
			continue
		}
		fmt.Fprintf(os.Stderr, "invalid argument: %s\n", arg)
		os.Exit(1)
	}
}
