// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build !js

package controlhttp

import (
	"crypto/tls"
	"net/http"

	"github.com/coder/websocket"
)

// websocketDialOptions preserves the caller's system dialer and trusted CAs.
// In particular, relay/control traffic must stay underneath the tailnet even
// when application traffic is using a userspace exit node.
func (d *Dialer) websocketDialOptions() (*websocket.DialOptions, func()) {
	proxy := d.getProxyFunc()
	if proxy == nil {
		proxy = http.ProxyFromEnvironment
	}
	client := &http.Client{Transport: &http.Transport{
		Proxy:           proxy,
		DialContext:     d.Dialer,
		TLSClientConfig: &tls.Config{RootCAs: d.ExtraRootCAs},
	}}
	return &websocket.DialOptions{HTTPClient: client}, client.CloseIdleConnections
}
