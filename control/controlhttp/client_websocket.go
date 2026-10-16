// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package controlhttp

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"net/url"

	"github.com/coder/websocket"
	"tailscale.com/control/controlbase"
	"tailscale.com/control/controlhttp/controlhttpcommon"
	"tailscale.com/net/wsconn"
)

// dialWebsocket carries the Noise protocol over a standard WebSocket upgrade.
func (d *Dialer) dialWebsocket(ctx context.Context) (*ClientConn, error) {
	if d.Hostname == "" {
		return nil, errors.New("required Dialer.Hostname empty")
	}

	init, cont, err := controlbase.ClientDeferred(d.MachineKey, d.ControlKey, d.ProtocolVersion)
	if err != nil {
		return nil, err
	}

	wsScheme := "wss"
	host := net.JoinHostPort(d.Hostname, strDef(d.HTTPSPort, "443"))
	// If using a custom control server (on a non-standard port), prefer that.
	// This mirrors the port selection in newNoiseClient from noise.go.
	// Also use ws:// when HTTPS is explicitly disabled (NoPort), which happens
	// for http:// URLs with private hostnames (e.g. http://localhost:31544).
	if d.HTTPSPort == NoPort || d.HTTPPort != "" && d.HTTPPort != "80" && d.HTTPSPort == "443" {
		wsScheme = "ws"
		host = net.JoinHostPort(d.Hostname, strDef(d.HTTPPort, "80"))
	}
	wsURL := &url.URL{
		Scheme: wsScheme,
		Host:   host,
		Path:   serverUpgradePath,
		// Send the handshake as a query parameter, which also works for
		// browser clients that cannot set arbitrary HTTP headers.
		RawQuery: url.Values{
			controlhttpcommon.HandshakeHeaderName: []string{base64.StdEncoding.EncodeToString(init)},
		}.Encode(),
	}
	opts, cleanup := d.websocketDialOptions()
	defer cleanup()
	opts.Subprotocols = []string{controlhttpcommon.UpgradeHeaderValue}
	wsConn, _, err := websocket.Dial(ctx, wsURL.String(), opts)
	if err != nil {
		return nil, err
	}
	netConn := wsconn.NetConn(context.Background(), wsConn, websocket.MessageBinary, wsURL.String())
	cbConn, err := cont(ctx, netConn)
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return &ClientConn{Conn: cbConn}, nil
}
