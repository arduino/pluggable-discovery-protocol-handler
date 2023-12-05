//
// This file is part of pluggable-discovery-protocol-handler.
//
// Copyright 2022 ARDUINO SA (http://www.arduino.cc/)
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

	"github.com/arduino/go-paths-helper"
	"github.com/stretchr/testify/require"
)

func TestDisc(t *testing.T) {
	builder, err := paths.NewProcess(nil, "go", "build")
	require.NoError(t, err)
	builder.SetDir("dummy-discovery")
	require.NoError(t, builder.Run())

	discovery, err := paths.NewProcess(nil, "./dummy-discovery")
	require.NoError(t, err)
	discovery.SetDir("dummy-discovery")

	stdout, err := discovery.StdoutPipe()
	require.NoError(t, err)
	stdin, err := discovery.StdinPipe()
	require.NoError(t, err)

	require.NoError(t, discovery.Start())

	{
		// Check that discovery is able to handle an "hello" without parameters gracefully
		// https://github.com/arduino/pluggable-discovery-protocol-handler/issues/32
		inN, err := stdin.Write([]byte("hello\n"))
		require.NoError(t, err)
		require.Greater(t, inN, 0)

		output := [1024]byte{}
		outN, err := stdout.Read(output[:])
		require.Greater(t, outN, 0)
		require.NoError(t, err)
		require.Equal(t, "{\n  \"eventType\": \"hello\",\n  \"message\": \"Invalid HELLO command\",\n  \"error\": true\n}\n", string(output[:outN]))
	}

	{
		inN, err := stdin.Write([]byte("quit\n"))
		require.NoError(t, err)
		require.Greater(t, inN, 0)

		output := [1024]byte{}
		outN, err := stdout.Read(output[:])
		require.Greater(t, outN, 0)
		require.NoError(t, err)
		require.Equal(t, "{\n  \"eventType\": \"quit\",\n  \"message\": \"OK\"\n}\n", string(output[:outN]))
	}
}
