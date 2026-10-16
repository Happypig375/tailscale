// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package localtcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEndpointLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "localapi.json")
	ln, token, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	ep, err := Read(path)
	if err != nil || ep.Token != token || ep.Address != ln.Addr().String() {
		t.Fatalf("endpoint read failed or mismatched: %v", err)
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal("endpoint is not mode 0600")
	}
	if other, _, err := Listen(path); err == nil {
		other.Close()
		t.Fatal("allowed two daemons to overwrite the same endpoint")
	}
	ln.Close()
	restarted, newToken, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if newToken == token {
		t.Fatal("restart reused an authentication token")
	}
	if _, err := Read(path); err != nil {
		t.Fatal(err)
	}
}

func TestRejectUnsafeEndpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "localapi.json")
	for _, address := range []string{"192.0.2.1:1234", "localhost:1234", "127.0.0.1:0"} {
		if err := writeEndpoint(path, Endpoint{Address: address, Token: strings.Repeat("a", 64)}); err != nil {
			t.Fatal(err)
		}
		if _, err := Read(path); err == nil {
			t.Fatalf("accepted unsafe destination %q", address)
		}
	}
	if err := writeEndpoint(path, Endpoint{Address: "127.0.0.1:1234", Token: "short"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil {
		t.Fatal("accepted a malformed token")
	}
	if err := writeEndpoint(path, Endpoint{Address: "127.0.0.1:1234", Token: strings.Repeat("a", 64)}); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil {
		t.Fatal("accepted publicly readable credentials")
	}
}
