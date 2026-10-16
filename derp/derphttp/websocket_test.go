// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build (linux || darwin) && ts_debug_websockets

package derphttp_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"tailscale.com/derp"
	"tailscale.com/derp/derphttp"
	"tailscale.com/derp/derpserver"
	"tailscale.com/envknob"
	"tailscale.com/net/netmon"
	"tailscale.com/types/key"
)

func TestWebsocketConnectCancellation(t *testing.T) {
	envknob.SetenvForTest(t, "TS_DEBUG_DERP_WS_CLIENT", "true")
	upgraded := make(chan struct{})
	release := make(chan struct{})
	releaseServer := sync.OnceFunc(func() { close(release) })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"derp"}})
		if err != nil {
			t.Error(err)
			return
		}
		defer ws.CloseNow()
		ws.CloseRead(r.Context())
		pingCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := ws.Ping(pingCtx); err != nil {
			t.Error(err)
			return
		}
		// A pong proves the client has finished upgrading and is reading
		// WebSocket frames while waiting for the DERP greeting.
		close(upgraded)
		// Stall the DERP greeting.
		<-release
	}))
	defer srv.Close()
	defer releaseServer()
	c, err := derphttp.NewClient(key.NewNode(), srv.URL+"/derp", t.Logf, netmon.NewStatic())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		releaseServer()
		c.Close()
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- c.Connect(ctx) }()
	select {
	case <-upgraded:
	case <-time.After(3 * time.Second):
		t.Fatal("WebSocket upgrade timed out")
	}
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("stalled handshake succeeded")
		}
	case <-time.After(time.Second):
		// Release the server before cleanup waits for Client.Close's mutex.
		releaseServer()
		<-result
		t.Fatal("Connect ignored cancellation after WebSocket upgrade")
	}
}

func TestWebsocketFailedHandshakeClosesConnection(t *testing.T) {
	envknob.SetenvForTest(t, "TS_DEBUG_DERP_WS_CLIENT", "true")
	peerClosed := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"derp"}})
		if err != nil {
			t.Error(err)
			return
		}
		defer ws.CloseNow()
		closed := ws.CloseRead(r.Context())
		// A zero-length DERP frame is not a valid server greeting.
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := ws.Write(ctx, websocket.MessageBinary, make([]byte, derp.FrameHeaderLen)); err != nil {
			t.Error(err)
			return
		}
		select {
		case <-closed.Done():
			close(peerClosed)
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)
	c, err := derphttp.NewClient(key.NewNode(), srv.URL+"/derp", t.Logf, netmon.NewStatic())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err == nil {
		t.Fatal("accepted an invalid DERP greeting")
	}
	select {
	case <-peerClosed:
	case <-time.After(time.Second):
		t.Fatal("failed handshake left its WebSocket connection open")
	}
}

func TestWebsocketSendRecvMaxPacket(t *testing.T) {
	envknob.SetenvForTest(t, "TS_DEBUG_DERP_WS_CLIENT", "true")
	s := derpserver.New(key.NewNode(), t.Logf)
	defer s.Close()
	srv := httptest.NewServer(derpserver.AddWebSocketSupport(s, derpserver.Handler(s)))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var clients [2]*derphttp.Client
	var keys [2]key.NodePublic
	for i := range clients {
		private := key.NewNode()
		keys[i] = private.Public()
		c, err := derphttp.NewClient(private, srv.URL+"/derp", t.Logf, netmon.NewStatic())
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		// Recv has no context argument; bound it by closing the client.
		stop := context.AfterFunc(ctx, func() { c.Close() })
		defer stop()
		if err := c.Connect(ctx); err != nil {
			t.Fatal(err)
		}
		waitConnect(t, c)
		clients[i] = c
	}
	for _, size := range []int{128, derp.MaxPacketSize} {
		for from, sender := range clients {
			to := 1 - from
			payload := bytes.Repeat([]byte{byte(from + 1)}, size)
			if err := sender.Send(keys[to], payload); err != nil {
				t.Fatalf("send %d bytes from client %d: %v", size, from, err)
			}
			for {
				msg, err := clients[to].Recv()
				if err != nil {
					t.Fatalf("receive %d bytes on client %d: %v", size, to, err)
				}
				if packet, ok := msg.(derp.ReceivedPacket); ok {
					if packet.Source != keys[from] || !bytes.Equal(packet.Data, payload) {
						t.Fatalf("received incorrect packet on client %d", to)
					}
					break
				}
			}
		}
	}
}
