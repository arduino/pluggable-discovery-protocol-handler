// This file is part of pluggable-discovery-protocol-handler.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

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
