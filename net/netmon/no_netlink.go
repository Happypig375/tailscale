// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package netmon

import (
	"net"
	"runtime"

	"tailscale.com/envknob"
)

var noNetlink = envknob.RegisterBool("TS_NO_NETLINK")

// NetlinkDisabled reports whether Linux userspace networking should avoid
// netlink, including the implicit netlink calls in Go's interface enumeration.
// This mode cannot be used with a kernel TUN or system route management.
func NetlinkDisabled() bool { return runtime.GOOS == "linux" && noNetlink() }

const noNetlinkInterfaceName = "userspace"

// interfacesWithoutNetlink discovers source addresses without enumerating
// interfaces. UDP connect only selects a route; it sends no packets. This sees
// only the default-route address for each family, not every local interface.
// An offline machine returns an empty address list so monitoring can start and
// detect connectivity on a subsequent poll.
func interfacesWithoutNetlink() ([]Interface, error) {
	return probeInterfaces(net.DialUDP), nil
}

func probeInterfaces(dial func(string, *net.UDPAddr, *net.UDPAddr) (*net.UDPConn, error)) []Interface {
	addrs := make([]net.Addr, 0, 2)
	for _, target := range []struct {
		network string
		ip      net.IP
	}{
		{"udp4", net.IPv4(8, 8, 8, 8)},
		{"udp6", net.ParseIP("2001:4860:4860::8888")},
	} {
		c, err := dial(target.network, nil, &net.UDPAddr{IP: target.ip, Port: 53})
		if err != nil {
			continue
		}
		ip := c.LocalAddr().(*net.UDPAddr).AddrPort().Addr().Unmap()
		c.Close()
		if !ip.IsGlobalUnicast() {
			continue
		}
		addrs = append(addrs, &net.IPNet{IP: ip.AsSlice(), Mask: net.CIDRMask(ip.BitLen(), ip.BitLen())})
	}
	return []Interface{{
		Interface: &net.Interface{Name: noNetlinkInterfaceName, MTU: 1500, Flags: net.FlagUp | net.FlagRunning},
		AltAddrs:  addrs,
	}}
}
