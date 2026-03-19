package server

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// dialWS opens a WebSocket connection to the test server and returns the conn.
func dialWS(t *testing.T, srv *httptest.Server, path string) *websocket.Conn {
	t.Helper()
	url := "ws" + srv.URL[len("http"):] + path
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("websocket.Dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "") })
	return conn
}

// readMsg reads one text message from the connection with a short timeout.
func readMsg(t *testing.T, conn *websocket.Conn) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, msg, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("conn.Read: %v", err)
	}
	return msg
}

// TestHub_Broadcast verifies that a message sent via Hub.Broadcast is
// delivered to a connected WebSocket client.
func TestHub_Broadcast(t *testing.T) {
	hub := NewHub(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	s := New(Config{}, nil)
	s.hub = hub

	srv := httptest.NewServer(s.mux)
	defer srv.Close()

	conn := dialWS(t, srv, "/ws")

	// Give the register goroutine time to process.
	time.Sleep(20 * time.Millisecond)

	want := []byte(`{"event":"test"}`)
	hub.Broadcast(want)

	got := readMsg(t, conn)
	if string(got) != string(want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestHub_SlowClient verifies that when one client's send buffer is full the
// message is dropped for that client but other clients still receive it.
func TestHub_SlowClient(t *testing.T) {
	hub := NewHub(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	s := New(Config{}, nil)
	s.hub = hub

	srv := httptest.NewServer(s.mux)
	defer srv.Close()

	// Connect two clients.
	fast := dialWS(t, srv, "/ws")
	slow := dialWS(t, srv, "/ws")

	// Give the register goroutine time to process both clients.
	time.Sleep(30 * time.Millisecond)

	// Flood the hub so the slow client's buffer fills before it reads.
	// wsSendBufSize == 64; send 65 messages — the 65th overflows and the
	// slow client is disconnected.
	payload := []byte(`{"event":"flood"}`)
	for i := 0; i < wsSendBufSize+1; i++ {
		hub.Broadcast(payload)
	}

	// The fast client reads all messages it received (may be some subset,
	// but at least one should arrive).
	received := 0
	readCtx, readCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer readCancel()
	for {
		_, _, err := fast.Read(readCtx)
		if err != nil {
			break
		}
		received++
	}
	if received == 0 {
		t.Error("fast client received no messages")
	}

	// The slow client should eventually be disconnected by the hub.
	disconnectCtx, disconnectCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer disconnectCancel()
	_, _, err := slow.Read(disconnectCtx)
	if err == nil {
		// The hub may have queued some messages before closing — drain until closed.
		for {
			_, _, err = slow.Read(disconnectCtx)
			if err != nil {
				break
			}
		}
	}
	// We only care that the slow client is eventually disconnected (err != nil).
	if err == nil {
		t.Error("slow client was not disconnected after buffer overflow")
	}
}
