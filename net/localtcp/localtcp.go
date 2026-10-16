// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

// Package localtcp implements private-file discovery for authenticated loopback
// LocalAPI connections on systems that cannot create Unix sockets.
package localtcp

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
)

// Endpoint is stored at the path selected by the usual --socket flag.
// Possession of Token grants administrative access to the daemon.
type Endpoint struct {
	Address string `json:"address"`
	Token   string `json:"token"`
}

// Listen binds exclusively to IPv4 loopback and atomically publishes a fresh
// secret in a mode-0600 file. The endpoint file is replaced on every restart.
func Listen(path string) (net.Listener, string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, "", err
	}
	lock, err := lockEndpoint(path + ".lock")
	if err != nil {
		return nil, "", err
	}
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		lock.Close()
		return nil, "", err
	}
	ln = &lockedListener{Listener: ln, lock: lock}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		ln.Close()
		return nil, "", err
	}
	ep := Endpoint{Address: ln.Addr().String(), Token: hex.EncodeToString(secret[:])}
	if err := writeEndpoint(path, ep); err != nil {
		ln.Close()
		return nil, "", err
	}
	return ln, ep.Token, nil
}

type lockedListener struct {
	net.Listener
	lock *os.File
}

func (ln *lockedListener) Close() error {
	err := ln.Listener.Close()
	ln.lock.Close()
	return err
}

func writeEndpoint(path string, ep Endpoint) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".localapi-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := json.NewEncoder(f).Encode(ep); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// Read reads a private endpoint file, refusing non-loopback destinations so
// that corrupt or malicious discovery data cannot send credentials off-host.
func Read(path string) (Endpoint, error) {
	f, err := os.Open(path)
	if err != nil {
		return Endpoint{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Endpoint{}, err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
		return Endpoint{}, fmt.Errorf("LocalAPI endpoint %q must be a private regular file (mode 0600)", path)
	}
	var ep Endpoint
	if err := json.NewDecoder(f).Decode(&ep); err != nil {
		return Endpoint{}, fmt.Errorf("read LocalAPI endpoint: %w", err)
	}
	addr, err := netip.ParseAddrPort(ep.Address)
	if err != nil || !addr.Addr().IsLoopback() || addr.Port() == 0 || len(ep.Token) != 64 {
		return Endpoint{}, fmt.Errorf("invalid LocalAPI endpoint in %q", path)
	}
	return ep, nil
}
