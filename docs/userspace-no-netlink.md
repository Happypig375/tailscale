# Restricted Linux networking with the upstream daemon and CLI

Build both `cmd/tailscaled` and `cmd/tailscale` with
`-tags=ts_restricted,ts_debug_websockets`. This replaces the former standalone
`tsnet-sandbox` launcher with the ordinary daemon and CLI lifecycle:

```sh
./tailscaled
# In another terminal:
./tailscale up
./tailscale status
./tailscale down
```

The restricted daemon enables `TS_NO_NETLINK`, DERP-only networking, control and
DERP WebSockets, the existing assumption that the network is available despite
failed interface probes, and disabled log uploads. HTTP proxy settings are
inherited. WebSockets preserve Noise authentication and encryption. Direct UDP
connections are unavailable. Untagged builds retain ordinary behavior.

Native control WebSockets honor the configured system dialer, additional trusted
CA roots, and custom HTTPS ports. They retain TLS certificate verification and
the inner Noise handshake. HTTP-only control servers use their configured HTTP
port (80 when omitted), including IPv6 literal addresses. Browser clients keep
using the browser's network and certificate settings.

The WebSocket byte-stream adapter leaves packet-size enforcement to DERP and
Noise, allowing DERP's full 64 KiB packets in either direction. Canceling a
connection attempt also interrupts a stalled DERP greeting after the WebSocket
upgrade, and failed handshakes close their upgraded connection.

## LocalAPI transport and storage

The usual `--socket` argument selects a private endpoint file instead of a Unix
socket. The daemon binds only to IPv4 loopback and atomically writes an address
and fresh random authentication token with mode `0600`. An advisory lock prevents
two daemons from publishing the same endpoint simultaneously. Every HTTP request
requires the token and `Sec-Tailscale: localapi`; browser-origin requests are
rejected. Authenticated clients have administrative access to this daemon.

The CLI reads the endpoint and supplies authentication internally, including for
streaming LocalAPI requests. Users run ordinary commands, not curl wrappers.
Non-loopback endpoint addresses and publicly readable credential files are
rejected. A stale file after shutdown does not authenticate to a restarted
instance; each daemon start generates a new token.

Default state is `tailscale-sandbox-state` beside the executable, and the default
endpoint is `localapi.json` within that directory. Keep both binaries together.
Use standard `--state`, `--statedir`, and `--socket` flags to override paths.
Changing `--statedir` does not implicitly change `--socket`. The selected storage
must be writable; there is no temporary fallback. Retain the full state directory
from the old launcher to preserve node identity; no migration runs automatically.
The old launcher and its `connection.json` are no longer used.

The daemon stays in the foreground or can be managed by an available supervisor.
When attached to an interactive terminal and a login URL is available, `c` copies
it and Enter invokes `xdg-open`. Local clipboard utilities are used when
available, otherwise OSC 52 is sent to a supporting terminal. Shortcuts do not
consume service input. Terminal settings are restored on login completion and
shutdown, and Ctrl+C retains its normal behavior. Login URLs are logged only
when they change.

## Application proxy versus environment gateway

The restricted build defaults `--outbound-http-proxy-listen` to
`127.0.0.1:1055`. Applications enter the tailnet through this local listener. Its
requests are dialed through UserDial and the userspace network stack, including
selected exit-node routes. HTTP and HTTPS CONNECT both follow that path. The
standard flag can select a different address or disable the listener with an
empty value; untagged builds still default to no HTTP listener. The usual
`--socks5-server` remains optional and can share the HTTP listener address.

The environment's `HTTPS_PROXY` is a separate outbound gateway for the daemon's
encrypted control and DERP WebSocket connections. For this host, `/etc/environment`
specifies `http://browser-proxy:8889`, loopback-only NO_PROXY exclusions, and a
Go CA certificate path. Restricted startup imports missing proxy and Go CA
settings from that file before network initialization. An explicit process value
in either case takes precedence over the entire upper/lower-case pair, including
empty values. Only recognized proxy and CA variables are imported; shell code
and variable expansions are never executed. Proxy credentials are not logged by
this loader.

Point Chromium at the local Tailscale listener without changing the daemon's
inherited environment gateway:

```sh
./tailscale set --exit-node=YOUR-EXIT-NODE
chromium --user-data-dir=/workspace/tailscale-chromium --proxy-server=http://127.0.0.1:1055
```

A separate profile prevents an existing Chromium process from discarding the
proxy flags. A desktop window using the environment proxy directly bypasses
Tailscale, even when an exit node is selected. Userspace mode does not intercept
host traffic or change browser configuration automatically.

Application proxies are separate from the management endpoint and do not inherit
its token authentication; keep them on loopback. Features requiring unavailable
kernel APIs remain unavailable even though their CLI commands are exposed.

Signed-in validation in the restricted command environment completed peer
discovery through DERP and exchanged TSMP and ICMP traffic with online peers.
The persistent node identity was reused across daemon restarts without another
authorization. The command environment's managed proxy required individual
relay-host approvals; its network namespace could not reach the host environment
gateway directly. These restrictions are separate from the outside terminal's
network access. Application proxy traffic and exit-node browsing were not part
of this signed-in validation.

## Netlink-only restrictions with ordinary builds

For environments that permit Unix and UDP sockets, untagged builds can still
use the narrower opt-in mode:

```sh
TS_NO_NETLINK=1 tailscaled --tun=userspace-networking \
  --state=/workspace/tailscale-state/tailscaled.state --socket=/workspace/tailscale-state/tailscaled.sock \
  --socks5-server=127.0.0.1:1055
tailscale --socket=/workspace/tailscale-state/tailscaled.sock up
```

The daemon rejects `TS_NO_NETLINK` with a kernel TUN configuration. Interface
discovery uses UDP connect without sending packets to obtain one default-route
source address per family on a synthetic `userspace` interface. Address slices
remain non-nil even offline to avoid implicit netlink enumeration. Monitoring
polls, normally every ten seconds on Linux. Default-route discovery uses the
synthetic interface; gateway/port-mapping discovery and routing-table dumps are
unavailable. Multi-interface and IPv6 link-local addresses are not enumerated.
