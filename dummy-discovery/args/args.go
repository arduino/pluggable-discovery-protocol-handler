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

package args

import (
	"fmt"
	"os"
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
		fmt.Fprintf(os.Stderr, "invalid argument: %s\n", arg)
		os.Exit(1)
	}
}
