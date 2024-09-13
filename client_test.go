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
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/arduino/go-paths-helper"
	"github.com/stretchr/testify/require"
)

type testLogger struct{}

func (l *testLogger) Debugf(msg string, args ...any) {
	fmt.Printf(msg, args...)
	fmt.Println()
}

func (l *testLogger) Errorf(msg string, args ...any) {
	fmt.Printf(msg, args...)
	fmt.Println()
}

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
	disc.SetLogger(&testLogger{})
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

func TestClient(t *testing.T) {
	// Build dummy-discovery
	builder, err := paths.NewProcess(nil, "go", "build")
	require.NoError(t, err)
	builder.SetDir("dummy-discovery")
	require.NoError(t, builder.Run())

	t.Run("WithDiscoveryCrashingOnStartup", func(t *testing.T) {
		// Run client with discovery crashing on startup
		cl := NewClient("1", "dummy-discovery/dummy-discovery", "--invalid")
		require.ErrorIs(t, cl.Run(), io.EOF)
	})

	t.Run("WithDiscoveryCrashingWhileSendingCommands", func(t *testing.T) {
		// Run client with crashing discovery after 1 second
		cl := NewClient("1", "dummy-discovery/dummy-discovery", "-k")
		require.NoError(t, cl.Run())

		time.Sleep(time.Second)

		ch, err := cl.StartSync(20)
		require.Error(t, err)
		require.Nil(t, ch)
	})

	t.Run("WithDiscoveryCrashingWhileStreamingEvents", func(t *testing.T) {
		// Run client with crashing discovery after 1 second
		cl := NewClient("1", "dummy-discovery/dummy-discovery", "-k")
		require.NoError(t, cl.Run())

		ch, err := cl.StartSync(20)
		require.NoError(t, err)

		time.Sleep(time.Second)

	loop:
		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					// Channel closed: Test passed
					fmt.Println("Event channel closed")
					break loop
				}
				fmt.Println("Recv: ", msg)
			case <-time.After(time.Second):
				t.Error("Crashing client did not close event channel")
				break loop
			}
		}

		cl.Quit()
	})
}
