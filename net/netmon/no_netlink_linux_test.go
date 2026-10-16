// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build !android

package netmon

import (
	"errors"
	"net"
	"testing"

	"tailscale.com/envknob"
	"tailscale.com/util/eventbus"
)

func TestNoNetlinkOffline(t *testing.T) {
	calls := 0
	ifs := probeInterfaces(func(network string, local, remote *net.UDPAddr) (*net.UDPConn, error) {
		calls++
		return nil, errors.New("network unreachable")
	})
	if calls != 2 || len(ifs) != 1 {
		t.Fatalf("got %d probes and %d interfaces", calls, len(ifs))
	}
	// In particular, the empty slice must be non-nil: nil makes Addrs
	// fall back to Go's netlink-based implementation.
	if ifs[0].AltAddrs == nil {
		t.Fatal("nil AltAddrs would fall back to netlink")
	}
	if addrs, err := ifs[0].Addrs(); err != nil || len(addrs) != 0 {
		t.Fatalf("offline Addrs = %v, %v", addrs, err)
	}
}

func TestNoNetlinkMonitor(t *testing.T) {
	envknob.SetenvForTest(t, "TS_NO_NETLINK", "true")
	// The explicit mode must bypass even a registered interface getter.
	old := altNetInterfaces
	altNetInterfaces = func() ([]Interface, error) {
		t.Fatal("called interface getter in no-netlink mode")
		return nil, nil
	}
	t.Cleanup(func() { altNetInterfaces = old })
	bus := eventbus.New()
	defer bus.Close()
	m, err := New(bus, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if _, ok := m.om.(*pollingMon); !ok {
		t.Fatalf("monitor = %T, want polling", m.om)
	}
	state := m.InterfaceState()
	if state.DefaultRouteInterface != noNetlinkInterfaceName {
		t.Fatalf("default interface = %q", state.DefaultRouteInterface)
	}
	if _, _, ok := likelyHomeRouterIPLinux(); ok {
		t.Fatal("unexpected gateway discovery")
	}
	m.Start()
}
