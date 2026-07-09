# Integration tests

[简体中文](README.zh-CN.md) | English

The automated virtual LAN lab lives in `tests/lab`. It uses the real macOS
`pf`, `dnsmasq`, and `mihomo` implementation with disposable Lima Linux clients.
The default loop covers NAT, DHCP/DNS, direct HTTPS, and explicit HTTPS through
mihomo `mixed-port`. The supported macOS transparent proxy path is TUN because
the current mihomo Darwin redir listener is unsupported.

CI currently runs the fast unit-test gate only. Run `make lab-test` locally, in
a nightly job, or as a manual macOS gate for changes that can alter real traffic
or host network state.

Policy-control changes can use a smaller non-root integration gate:

```sh
make policy-control-test
```

This gate writes an imported mihomo fixture under `runtime/integration/`, renders
OpenSurge's gateway overlay, starts the real mihomo binary without dnsmasq, pf,
TUN, or sudo, and verifies `omg policies`, `omg policy-select`, and
`omg connections` against the live external-controller API. It rejects an
unknown policy before switching, then restarts mihomo in the same runtime
directory to verify `profile.store-selected` restores the selected policy. It
proves the control-plane contract with mihomo, not whole-LAN routing or
transparent proxy capture.

The transparent proxy gate is `make lab-test-tun`. It is stricter than the
default lab path because clients do not use `mixed-port`; the test must prove
that mihomo observed the client HTTPS connection through TUN.

Real-device tests remain a separate milestone-level check for Wi-Fi behavior,
device-specific protocol quirks, and IPv6. Never enable the project's DHCP
server on a normal home or office LAN during integration testing. See
`tests/real-device/README.md` for the isolated downstream-LAN smoke plan.
