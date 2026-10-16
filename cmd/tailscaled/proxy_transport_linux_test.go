// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build linux && ts_restricted && ts_debug_websockets

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"tailscale.com/control/controlhttp"
	"tailscale.com/derp/derphttp"
	"tailscale.com/envknob"
	"tailscale.com/net/netmon"
	"tailscale.com/types/key"
)

func TestTransportUsesHTTPSProxy(t *testing.T) {
	if transport := os.Getenv("TS_TEST_PROXY_TRANSPORT"); transport != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var err error
		if transport == "control" {
			envknob.SetenvForTest(t, "TS_CONTROL_WEBSOCKET", "true")
			d := &controlhttp.Dialer{Hostname: "proxy-test.invalid", MachineKey: key.NewMachine(), ControlKey: key.NewMachine().Public(), ProtocolVersion: 1}
			_, err = d.Dial(ctx)
		} else {
			envknob.SetenvForTest(t, "TS_DEBUG_DERP_WS_CLIENT", "true")
			var c *derphttp.Client
			c, err = derphttp.NewClient(key.NewNode(), "https://proxy-test.invalid/derp", t.Logf, netmon.NewStatic())
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			err = c.Connect(ctx)
		}
		if err == nil {
			t.Fatal("ignored the proxy's rejection")
		}
		return
	}
	for _, transport := range []string{"control", "DERP"} {
		t.Run(transport, func(t *testing.T) {
			targets := make(chan string, 4)
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "CONNECT" {
					targets <- r.Host
				}
				http.Error(w, "test proxy refuses tunnel", http.StatusBadGateway)
			}))
			defer proxy.Close()
			// A fresh process avoids net/http's package-global environment cache.
			t.Setenv("HTTPS_PROXY", proxy.URL)
			t.Setenv("https_proxy", proxy.URL)
			t.Setenv("NO_PROXY", "")
			t.Setenv("no_proxy", "")
			t.Setenv("TS_TEST_PROXY_TRANSPORT", transport)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTransportUsesHTTPSProxy$")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("child test: %v\n%s", err, output)
			}
			select {
			case target := <-targets:
				if target != "proxy-test.invalid:443" {
					t.Fatalf("proxy CONNECT target = %q", target)
				}
			default:
				t.Fatal("transport bypassed HTTPS_PROXY")
			}
		})
	}
}
