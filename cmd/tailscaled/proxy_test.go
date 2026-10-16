// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build !ts_omit_outboundproxy

package main

import (
	"bufio"
	"context"
	"flag"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"tailscale.com/net/localtcp"
)

func TestOutboundHTTPProxyDefault(t *testing.T) {
	oldFlags, oldSOCKS, oldHTTP := flag.CommandLine, args.socksAddr, args.httpProxyAddr
	t.Cleanup(func() { flag.CommandLine, args.socksAddr, args.httpProxyAddr = oldFlags, oldSOCKS, oldHTTP })
	flag.CommandLine = flag.NewFlagSet("proxy-test", flag.ContinueOnError)
	registerOutboundProxyFlags()
	want := ""
	if localtcp.Enabled {
		want = "127.0.0.1:1055"
	}
	if args.httpProxyAddr != want {
		t.Fatalf("default HTTP listener = %q, want %q", args.httpProxyAddr, want)
	}
	if err := flag.CommandLine.Parse([]string{"--outbound-http-proxy-listen="}); err != nil || args.httpProxyAddr != "" {
		t.Fatal("cannot explicitly disable the listener")
	}
}

// Both HTTP requests and HTTPS CONNECT must enter UserDial, which selects the
// tailnet route (including the chosen exit node), rather than the environment's
// forward proxy. That gateway is used separately for encrypted DERP transport.
func TestHTTPProxyUsesUserDial(t *testing.T) {
	for _, connect := range []bool{false, true} {
		t.Run(map[bool]string{false: "HTTP", true: "CONNECT"}[connect], func(t *testing.T) {
			targets := make(chan string, 1)
			dial := func(ctx context.Context, network, address string) (net.Conn, error) {
				targets <- address
				client, server := net.Pipe()
				go func() {
					defer server.Close()
					if connect {
						io.Copy(server, server)
						return
					}
					if _, err := http.ReadRequest(bufio.NewReader(server)); err == nil {
						io.WriteString(server, "HTTP/1.1 200 OK\r\nContent-Length: 7\r\nConnection: close\r\n\r\ntailnet")
					}
				}()
				return client, nil
			}
			srv := httptest.NewServer(httpProxyHandler(dial))
			defer srv.Close()
			if connect {
				c, err := net.DialTimeout("tcp", srv.Listener.Addr().String(), time.Second)
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
				c.SetDeadline(time.Now().Add(3 * time.Second))
				io.WriteString(c, "CONNECT 203.0.113.8:443 HTTP/1.1\r\nHost: 203.0.113.8:443\r\n\r\n")
				reader := bufio.NewReader(c)
				resp, err := http.ReadResponse(reader, &http.Request{Method: "CONNECT"})
				if err != nil || resp.StatusCode != 200 {
					t.Fatalf("CONNECT failed: %v", err)
				}
				io.WriteString(c, "tailnet")
				buf := make([]byte, 7)
				if _, err := io.ReadFull(reader, buf); err != nil || string(buf) != "tailnet" {
					t.Fatalf("tunnel failed: %v", err)
				}
			} else {
				proxyURL, _ := url.Parse(srv.URL)
				transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
				defer transport.CloseIdleConnections()
				client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
				resp, err := client.Get("http://203.0.113.8:443/resource")
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				if err != nil || strings.TrimSpace(string(body)) != "tailnet" {
					t.Fatalf("proxy request failed: %v", err)
				}
			}
			select {
			case target := <-targets:
				if target != "203.0.113.8:443" {
					t.Fatalf("UserDial target = %q", target)
				}
			default:
				t.Fatal("request bypassed UserDial")
			}
		})
	}
}
