# Tailscale for restricted Linux cloud computers

Run **`tailscaled`** for the node and **`tailscale`** to manage it, using the
usual upstream commands. This Linux fork works in environments that deny
netlink, UDP sockets, Unix sockets, and kernel TUN devices.

**Applications must explicitly use Tailscale's local proxy to reach the tailnet
or use an exit node.** Selecting an exit node does not redirect the desktop
browser or all terminal programs automatically. This is how
[upstream userspace networking](https://tailscale.com/docs/concepts/userspace-networking)
works; this environment's restrictions prevent using normal TUN-based host
routing. See [Chromium, terminal programs, and exit nodes](#chromium-terminal-programs-and-exit-nodes).

## Install and run

Download `tailscaled`, `tailscale`, and `SHA256SUMS` from
[Releases](https://github.com/Happypig375/tailscale/releases/latest) into the same
writable directory. The release binaries target Linux amd64.

```sh
sha256sum -c SHA256SUMS
chmod +x tailscaled tailscale
./tailscaled
```

In another terminal:

```sh
./tailscale up
./tailscale status
./tailscale ip
./tailscale set --hostname=my-cloud-computer
./tailscale down
```

When login is needed, `up` prints a URL. In the daemon's foreground terminal,
press **c** to copy it or **Enter** to open it in the browser. The URL is shown
when it changes, without repeated reminders. Copying requires a local clipboard
utility or an OSC 52-capable terminal; opening requires `xdg-open` and a browser.
Ctrl+C stops the daemon. It can also run under a process supervisor.

## State and configuration

State is stored in `tailscale-sandbox-state` beside the executables. Keep that
directory when upgrading to retain your login. If you move the binaries, move
it with them. The directory must be writable; there is no temporary fallback.

The usual flags override paths:

```sh
./tailscaled --statedir=/workspace/tailscale-state --socket=/workspace/tailscale-state/localapi.json
./tailscale --socket=/workspace/tailscale-state/localapi.json status
```

`--socket` selects a private endpoint file for authenticated loopback TCP.
The CLI handles authentication automatically. Keep the state directory private.
Changing `--statedir` does not change `--socket` automatically.

## Differences from upstream

- Networking runs entirely in userspace, with polling instead of netlink.
- Control and DERP connections use WebSockets and inherited HTTP proxy settings
  such as `HTTPS_PROXY`. Noise authentication and encryption are preserved.
  Native control connections also honor custom HTTPS ports and trusted CA roots.
- Peer traffic uses DERP relays; direct UDP connections are unavailable.
  WebSocket relays support DERP's full 64 KiB packet size.
- Management uses authenticated loopback TCP internally instead of Unix sockets.
- State defaults beside the binaries, and log uploads are disabled.

## Chromium, terminal programs, and exit nodes

The restricted daemon listens for application HTTP/HTTPS proxy requests at
`127.0.0.1:1055` by default. After signing in, select an exit node normally:

```sh
./tailscale set --exit-node=YOUR-EXIT-NODE
```

Start a Chromium window that uses the local listener:

```sh
chromium --user-data-dir=/workspace/tailscale-chromium --proxy-server=http://127.0.0.1:1055
```

The separate profile ensures an already-running Chromium process does not ignore
these flags. Use this window for exit-node browsing; an existing desktop window
keeps its previous proxy settings.

The traffic path is:

**Chromium → local Tailscale HTTP listener → encrypted DERP connection through
the environment's HTTPS_PROXY → exit node → destination.**

In this environment, `/etc/environment` configures `browser-proxy:8889` as the
outbound gateway. Keep that gateway configured for the daemon. Do not globally
replace `HTTPS_PROXY` with Tailscale's own listener: Chromium's explicit proxy
flag selects the application entry point without changing the daemon's gateway.

Userspace mode does not install host routes. Selecting an exit node alone does
not redirect the ordinary desktop browser, which otherwise continues to use the
environment gateway directly. Other applications must also use the local
listener, for example:

```sh
curl --proxy http://127.0.0.1:1055 https://example.com
```

Terminal programs follow the same rule as Chromium. Programs that honor proxy
environment variables can be configured for a single command:

```sh
HTTP_PROXY=http://127.0.0.1:1055 http_proxy=http://127.0.0.1:1055 \
HTTPS_PROXY=http://127.0.0.1:1055 https_proxy=http://127.0.0.1:1055 \
your-program
```

Programs that ignore HTTP proxy settings need SOCKS5 support or another adapter.
Merely launching a program from a terminal does not route it through Tailscale.
Keep these settings scoped to the application, so the daemon continues using
the environment's outbound gateway. The local HTTP listener is enabled by this
fork; the need to configure applications comes from upstream userspace mode.

The daemon reads missing HTTP, HTTPS, ALL_PROXY, NO_PROXY, and Go CA certificate
settings from `/etc/environment`. Explicit process settings take precedence,
including empty values. Standard `NO_PROXY` exclusions still apply to the
underlying relay connections. No shell code from the file is executed.

Use `--outbound-http-proxy-listen=127.0.0.1:OTHER_PORT` to change the listener, or
`--outbound-http-proxy-listen=` to disable it. The standard `--socks5-server`
option remains available. Application listeners have no password and should
remain bound to loopback; management uses its separate authenticated endpoint.
Features requiring kernel routing or other blocked facilities remain unavailable.
Signed-in DERP traffic has been verified in this environment using peer discovery,
TSMP, and ICMP. The node retained its signed-in identity across daemon restarts.

## Build and maintain

Use Go 1.27.2 or later:

```sh
go build -tags=ts_restricted,ts_debug_websockets -o ./dist/tailscaled ./cmd/tailscaled
go build -tags=ts_restricted,ts_debug_websockets -o ./dist/tailscale ./cmd/tailscale
```

Both binaries must use `ts_restricted`; `ts_debug_websockets` includes native
DERP WebSockets. Builds without these tags retain upstream behavior.

The initial restricted release establishes the fork's baseline commit. Make
future updates as ordinary commits and releases, preserving history, tags, and
previous releases. The one-time squash-and-replace release script has been
removed and must not be used for subsequent updates.

Before releasing, run the documented validation, review the changed files, and
commit the source. Build both binaries from that commit with the tags above,
generate `SHA256SUMS`, and publish those three assets under a new release tag.
The target is `Happypig375/tailscale`, branch `dot-cloud-computer`.

The local installation is `/workspace/shared`. Stop its daemon before replacing
the executables, preserve `tailscale-sandbox-state`, and restart the daemon
afterward. Keep both binaries executable and together beside that state directory.

See [implementation details](docs/userspace-no-netlink.md) and
[agent instructions](AGENTS.md) for maintenance and validation.

## Upstream project

This fork is based on [Tailscale](https://tailscale.com), primarily developed by
[Tailscale](https://github.com/tailscale) and its
[contributors](https://github.com/tailscale/tailscale/graphs/contributors).
Upstream packages and documentation are available at
[tailscale.com](https://tailscale.com) and do not include this fork's changes.
Contributions require a `Signed-off-by` line; see
[commit message conventions](docs/commit-messages.md).

WireGuard is a registered trademark of Jason A. Donenfeld.
