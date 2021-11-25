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
	"testing"

	"github.com/arduino/arduino-cli/executils"
	"github.com/stretchr/testify/require"
)

func runDummyDiscovery(t *testing.T) *executils.Process {
	discoveryDir := "dummy-discovery"
	// Build dummy-discovery for testing
	builder, err := executils.NewProcess("go", "build")
	require.NoError(t, err)
	builder.SetDir(discoveryDir)
	require.NoError(t, builder.Run())
	discovery, err := executils.NewProcess("./dummy-discovery")
	require.NoError(t, err)
	discovery.SetDir(discoveryDir)
	return discovery
}

func TestDiscoveryStdioHandling(t *testing.T) {
	discovery := runDummyDiscovery(t)

	stdout, err := discovery.StdoutPipe()
	require.NoError(t, err)
	stdin, err := discovery.StdinPipe()
	require.NoError(t, err)

	require.NoError(t, discovery.Start())
	defer discovery.Kill()

	n, err := stdin.Write([]byte("quit\n"))
	require.Greater(t, n, 0)
	require.NoError(t, err)
	output := [1024]byte{}
	// require.NoError(t, discovery.Wait())
	n, err = stdout.Read(output[:])
	require.Greater(t, n, 0)
	require.NoError(t, err)

	expectedOutput := "{\n  \"eventType\": \"quit\",\n  \"message\": \"OK\"\n}\n"
	require.Equal(t, expectedOutput, string(output[:n]))
}
