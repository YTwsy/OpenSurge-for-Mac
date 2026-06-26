# Integration tests

The automated virtual LAN lab lives in `tests/lab`. It uses the real macOS
`pf`, `dnsmasq`, and `mihomo` implementation with disposable Lima Linux clients.
The default loop covers NAT, DHCP/DNS, direct HTTPS, and explicit HTTPS through
mihomo `mixed-port`; transparent TCP redirection is not enabled by default on
macOS because the current mihomo Darwin redir listener is unsupported.

CI currently runs the fast unit-test gate only. Run `make lab-test` locally, in
a nightly job, or as a manual macOS gate for changes that can alter real traffic
or host network state.

The experimental transparent proxy gate is `make lab-test-tun`. It is stricter
than the default lab path because clients do not use `mixed-port`; the test must
prove that mihomo observed the client HTTPS connection through TUN.

Real-device tests remain a separate milestone-level check for Wi-Fi behavior,
device-specific protocol quirks, and IPv6. Never enable the project's DHCP
server on a normal home or office LAN during integration testing.
