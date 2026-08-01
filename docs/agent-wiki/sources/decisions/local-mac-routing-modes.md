# Decision: local Mac routing modes are a source-scoped overlay

Status: accepted

OpenSurge exposes Rule, Global, and Direct for traffic originating on the
gateway Mac without changing mihomo's top-level `mode: rule`.

The generated overlay owns these hidden selector names:

- `open-surge/mac-global`
- `open-surge/mac-mode-tcp`
- `open-surge/mac-mode-udp`

Local rules must match both inbound type and source identity. TUN traffic uses
`IN-TYPE,TUN` plus `SRC-IP-CIDR,198.18.0.1/32`. Local explicit proxy traffic
uses `IN-TYPE,SOCKS/HTTP` plus either loopback or the gateway LAN IPv4. These
rules precede device and imported rules. Downstream device source addresses
must not match them.

Rule selects `PASS` for both TCP and UDP so evaluation continues through the
normal gateway rules. Direct selects `DIRECT`. Global selects
`open-surge/mac-global` for TCP. UDP uses the same selector only when the
selected live target reports UDP support; otherwise it selects `REJECT`.

Local/private destination guards remain direct before the mode dispatch.
Switches affect new connections and use mihomo's stored selector state for
restart persistence. The local-routing switch does not mutate macOS
system-proxy settings; the separate, explicitly enabled compatibility setting
is governed by `local-system-proxy-coordination.md`.

Generic policy APIs and CLI must hide and reject `open-surge/mac-*`; the
dedicated local-routing API owns coordinated TCP/UDP/global selection and
rollback.
