# Working on this fork

This branch supports Tailscale userspace networking in restricted Linux cloud
computers. Start with `README.md` and `docs/userspace-no-netlink.md`.

## Implementation

- `net/netmon/no_netlink.go` implements the opt-in `TS_NO_NETLINK` mode. Avoid
  implicit netlink calls through `net.Interfaces`, interface address enumeration,
  and interface index/name lookups when this mode is enabled.
- Keep synthetic interface `AltAddrs` non-nil even when offline; a nil slice
  invokes the standard library's interface enumeration.
- Publish only `tailscaled` and `tailscale`, built with
  `-tags=ts_restricted,ts_debug_websockets`. Preserve upstream command parsing and
  daemon lifecycle. Do not reintroduce a standalone launcher or curl-based CLI.
- `net/localtcp` implements private-file discovery and authenticated loopback IPC.
  Preserve per-request authentication, loopback binding, token rotation, endpoint
  locking, file permissions, and browser-origin protections.
- Restricted state defaults beside the executable; never use temporary storage
  as a fallback. `--state`, `--statedir`, and `--socket` remain explicit overrides.
- Login hotkeys apply only to interactive terminals while login is needed; restore
  terminal settings on shutdown and do not repeat unchanged login URLs.
- Keep application ingress separate from the environment gateway. Restricted
  builds expose HTTP ingress on `127.0.0.1:1055`; Chromium must explicitly use it.
  DERP/control use the daemon's HTTPS_PROXY underneath the tailnet. Do not send
  relay connections through UserDial or globally point the daemon at itself.
- Restricted startup fills missing proxy/CA settings from `/etc/environment`.
  Preserve explicit process overrides (including empty values), never execute
  the file as shell code, and never log its credentials.
- `TS_CONTROL_WEBSOCKET` selects the shared browser/native Noise-over-WebSocket
  transport. Preserve Noise authentication and encryption.
- Keep ordinary Tailscale behavior unchanged when the new settings are absent.
- Never commit auth keys, login URLs, connection credentials, generated node
  state, downloaded toolchains, or compiled binaries.

## Validation

Use Go 1.27.2 or later. If the system `go` is another program, invoke the Go
toolchain by its absolute path. Put `GOCACHE` and `GOMODCACHE` in writable
directories if their defaults are read-only.

```sh
go test ./net/netmon -run TestNoNetlink -count=1
go test ./control/controlhttp -run TestWebsocketDialPorts -count=1
go test ./control/controlhttp/controlhttpserver -count=1
go test -tags=ts_restricted,ts_debug_websockets ./derp/derphttp -run TestWebsocket -count=1
go test -tags=ts_restricted,ts_debug_websockets ./net/localtcp ./ipn/ipnserver -run 'TestEndpointLifecycle|TestRejectUnsafeEndpoint|TestLocalTCP' -count=1
go test -tags=ts_restricted,ts_debug_websockets ./cmd/tailscaled -run 'TestLoadProxyEnvironment|TestTransportUsesHTTPSProxy|TestHTTPProxyUsesUserDial|TestOutboundHTTPProxyDefault' -count=1
TS_NO_NETLINK=true TS_DEBUG_ALWAYS_USE_DERP=true TS_ASSUME_NETWORK_UP_FOR_TEST=true go test ./ipn/ipnlocal -run '^TestEnginePeerForIPAdjustsForPrefs$' -count=1
go build -tags=ts_restricted,ts_debug_websockets ./cmd/tailscaled
go build -tags=ts_restricted,ts_debug_websockets ./cmd/tailscale
go build ./cmd/tailscaled
go build ./cmd/tailscale
git diff --check
```

For a runtime check, build the two binaries together in a fresh writable test
directory. Verify default state is placed beside them, ordinary CLI status,
`up` login prompting, preference changes, `down`, and graceful shutdown. Check
that anonymous LocalAPI access is denied, authenticated access works, and
`localapi.json` is mode `0600`. Test token rotation and persisted machine identity
across restart, and terminal hotkeys with mocked clipboard/browser programs.
Stop all test processes and remove test state after validation. Do not claim
tailnet connectivity until a node has actually signed in and exchanged traffic.

The default netmon and routing-table suites require netlink and can fail in
this sandbox. Report these environmental failures separately from focused test
results. Do not change fixture expectations to accommodate sandbox restrictions.
The sandbox can also deny ptrace and writes to `.git`; report these limitations
instead of weakening authentication or altering system protections.

Before pushing, test the final source, review the changed files, and include
these agent instructions with the implementation and user documentation.
