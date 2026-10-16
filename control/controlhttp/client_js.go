// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package controlhttp

import (
	"context"

	"github.com/coder/websocket"
)

// The browser manages proxy settings, certificate trust, and network dialing.
func (d *Dialer) websocketDialOptions() (*websocket.DialOptions, func()) {
	return &websocket.DialOptions{}, func() {}
}

// Browsers require a WebSocket transport for bidirectional communication.
func (d *Dialer) Dial(ctx context.Context) (*ClientConn, error) {
	return d.dialWebsocket(ctx)
}
