// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package ipnserver

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

type authTestListener struct {
	net.Listener
	addr   net.Addr
	closed bool
}

func (ln *authTestListener) Addr() net.Addr { return ln.addr }
func (ln *authTestListener) Close() error   { ln.closed = true; return nil }

func TestLocalTCPRequiresLoopbackAndToken(t *testing.T) {
	for _, tt := range []struct {
		name, ip, token string
	}{
		{"wildcard", "0.0.0.0", "secret"},
		{"public address", "192.0.2.1", "secret"},
		{"empty token", "127.0.0.1", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ln := &authTestListener{addr: &net.TCPAddr{IP: net.ParseIP(tt.ip), Port: 1234}}
			if err := new(Server).RunWithLocalAPIToken(context.Background(), ln, tt.token); err == nil {
				t.Fatal("accepted an unsafe listener or missing token")
			}
			if !ln.closed {
				t.Fatal("did not close rejected listener")
			}
		})
	}
}

func TestLocalTCPAuthentication(t *testing.T) {
	s := new(Server)
	const token = "test-only-token"
	for _, tt := range []struct {
		name, password, header, origin string
		want                           int
	}{
		{name: "anonymous", want: http.StatusForbidden},
		{name: "header only", header: "localapi", want: http.StatusUnauthorized},
		{name: "wrong password", header: "localapi", password: "wrong", want: http.StatusUnauthorized},
		{name: "no header", password: token, want: http.StatusForbidden},
		{name: "browser origin", header: "localapi", password: token, origin: "https://example.com", want: http.StatusForbidden},
		// Authentication succeeds and reaches the not-yet-ready backend.
		{name: "authenticated", header: "localapi", password: token, want: http.StatusServiceUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://local-tailscaled.sock/server-status", nil)
			r.Header.Set("Sec-Tailscale", tt.header)
			r.Header.Set("Origin", tt.origin)
			if tt.password != "" {
				r.SetBasicAuth("", tt.password)
			}
			w := httptest.NewRecorder()
			s.localTCPHandler(token).ServeHTTP(w, r)
			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d", w.Code, tt.want)
			}
		})
	}
}
