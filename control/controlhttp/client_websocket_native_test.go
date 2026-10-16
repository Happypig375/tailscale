// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build !js

package controlhttp

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"testing"

	"tailscale.com/types/key"
)

func TestWebsocketDialPorts(t *testing.T) {
	for _, tt := range []struct {
		name, host, httpPort, httpsPort, want string
	}{
		{"defaults", "example.invalid", "", "", "example.invalid:443"},
		{"custom HTTPS", "example.invalid", "", "8443", "example.invalid:8443"},
		{"HTTP only", "example.invalid", "", NoPort, "example.invalid:80"},
		{"custom HTTP", "example.invalid", "8080", NoPort, "example.invalid:8080"},
		{"private HTTP", "example.invalid", "8080", "443", "example.invalid:8080"},
		{"IPv6", "::1", "", "8443", "[::1]:8443"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			stop := errors.New("test dial stopped")
			d := &Dialer{
				Hostname: tt.host, HTTPPort: tt.httpPort, HTTPSPort: tt.httpsPort,
				MachineKey: key.NewMachine(), ControlKey: key.NewMachine().Public(), ProtocolVersion: 1,
				proxyFunc: func(*http.Request) (*url.URL, error) { return nil, nil },
				Dialer: func(_ context.Context, _, address string) (net.Conn, error) {
					got = address
					return nil, stop
				},
			}
			_, err := d.dialWebsocket(context.Background())
			if !errors.Is(err, stop) || got != tt.want {
				t.Fatalf("dial = %q, %v; want %q and test error", got, err, tt.want)
			}
		})
	}
}
