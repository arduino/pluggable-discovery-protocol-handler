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

import (
	"net"
	"testing"
	"time"

	"github.com/arduino/go-paths-helper"
	"github.com/stretchr/testify/require"
)

func TestDiscoveryStdioHandling(t *testing.T) {
	// Build `netcat` helper inside testdata/cat
	builder, err := paths.NewProcess(nil, "go", "build")
	require.NoError(t, err)
	builder.SetDir("testdata/netcat")
	require.NoError(t, builder.Run())

	// Run netcat and test if streaming json works as expected
	listener, err := net.ListenTCP("tcp", nil)
	require.NoError(t, err)

	disc := NewClient("test", "testdata/netcat/netcat", listener.Addr().String())
	err = disc.runProcess()
	require.NoError(t, err)

	listener.SetDeadline(time.Now().Add(time.Second))
	conn, err := listener.Accept()
	require.NoError(t, err)

	_, err = conn.Write([]byte(`{ "eventType":`)) // send partial JSON
	require.NoError(t, err)
	msg, err := disc.waitMessage(time.Millisecond * 100)
	require.Error(t, err)
	require.Nil(t, msg)

	_, err = conn.Write([]byte(`"ev1" }{ `)) // complete previous json and start another one
	require.NoError(t, err)

	msg, err = disc.waitMessage(time.Millisecond * 100)
	require.NoError(t, err)
	require.NotNil(t, msg)
	require.Equal(t, "ev1", msg.EventType)

	msg, err = disc.waitMessage(time.Millisecond * 100)
	require.Error(t, err)
	require.Nil(t, msg)

	_, err = conn.Write([]byte(`"eventType":"ev2" }`)) // complete previous json
	require.NoError(t, err)

	msg, err = disc.waitMessage(time.Millisecond * 100)
	require.NoError(t, err)
	require.NotNil(t, msg)
	require.Equal(t, "ev2", msg.EventType)

	require.True(t, disc.Alive())

	err = conn.Close()
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 500)

	require.False(t, disc.Alive())
}
