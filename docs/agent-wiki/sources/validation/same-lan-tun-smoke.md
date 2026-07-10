---
title: same-LAN TUN smoke
kind: source
status: seed
---

# same-LAN TUN smoke

same-LAN TUN smoke is the first validation layer for the Surge-like default
gateway scenario where the Mac and a test Android device stay on the same
home or office LAN.

It is not the isolated downstream-LAN real-device smoke. It also is not an
explicit proxy check.

## Contract

The first supported same-LAN slice is:

- `gateway.mode: "same_lan"`;
- `gateway.interface` and `gateway.upstream_interface` refer to the same macOS
  LAN interface;
- `dhcp.enabled: false`;
- dnsmasq runs as DNS-only on the Mac LAN IP;
- DNS forwards to mihomo DNS at `127.0.0.1#1053`;
- `transparent.mode: "tun"`;
- pf NAT excludes the local LAN CIDR so same-LAN traffic is not intentionally
  NATed;
- one test Android device manually points default gateway and DNS to the Mac
  LAN IP.

Do not run OpenSurge DHCP on the main home or office LAN for this smoke. Do not
claim whole-home readiness from this gate.

For the same-WiFi DHCP takeover runner, where router DHCP is disabled and
OpenSurge serves DHCP/DNS on the same Wi-Fi, read
`tests/same-lan/WIFI-DHCP-RECOVERY.zh-CN.md` first. Recovery is part of the
validation contract: the router DHCP must be restored, the Mac must return to
DHCP, and at least one client must automatically obtain an address again.

## same-WiFi DHCP imported egress runner

The explicit high-risk mode is `gateway.mode: "same_wifi_dhcp"`; `same_lan`
continues to require DHCP disabled. The entrypoints are:

- `make same-wifi-dhcp-start-imported-egress`;
- `make same-wifi-dhcp-adb-check-imported-egress`;
- `make same-wifi-dhcp-stop`.

The start command requires `OMG_SAME_WIFI_DHCP_ROUTER_DHCP_DISABLED=confirmed`
after the operator manually disables router DHCP, plus a non-empty
`OMG_SAME_WIFI_DHCP_PROTECTED_IPS` list. The confirmation is a safety receipt,
not an automated router-state check. The runner requires the Mac Wi-Fi service
to remain manually addressed, defaults its range to `.120-.199` within that
`/24`, and refuses any range that includes the Mac gateway or a protected static
address.

The ADB gate must observe an Android address inside the new range in both
`omg leases` and a dnsmasq DHCPACK log, DNS source evidence, no Android explicit
proxy, live provider plus `provider-update`, TUN traffic through
`TunEgress[DIRECT]`, followed by `policy-select` to `egress-proxy` and a
controlled `CONNECT <host>:443` observation. Stop verifies removal of the
runtime state, PF anchor, listeners, and egress helper while restoring IPv4
forwarding. It deliberately does not re-enable router DHCP or return clients to
DHCP; those remain explicit recovery steps.

### 2026-07-11 manual-phone validation

On a dedicated test Wi-Fi, router DHCP was manually disabled while the Mac kept
the static address `192.168.1.20`. With `192.168.1.101` protected outside the
`.120-.199` lease pool, the controlled helper used that address's HTTP proxy as
its upstream to avoid re-entering TUN. A manually operated Android phone received
`192.168.1.141`; dnsmasq logged its `example.com` query. Browser probes produced
both `TunEgress[DIRECT]` and, after policy selection, `TunEgress[egress-proxy]`
with a controlled CONNECT observation. The operator confirmed browser access on
the latter path. `same-wifi-dhcp-stop` then removed the runtime state, services,
PF anchor, helper, and forwarding state. This is one-device, manual evidence;
it does not broaden the runner's compatibility or recovery claims.

## Runner

The runner is `tests/same-lan/smoke.sh` with Makefile entrypoints:

- `make same-lan-start-tun`;
- `make same-lan-start-tun-proxy`;
- `make same-lan-start-tun-imported-egress`;
- `make same-lan-adb-check`;
- `make same-lan-adb-check-imported-egress`;
- `make same-lan-status`;
- `make same-lan-stop`.

The runner writes `runtime/same-lan/config-tun.yaml`, builds `bin/omg`, starts
the same-LAN TUN config with sudo, and leaves Android-side proof to ADB probes.

## ADB proof

ADB is the preferred client automation layer. It should collect:

- authorized device state;
- Android IPv4 address and default route;
- default route containing `via <mac-lan-ip>`;
- DNS query against the Mac LAN IP for the test host;
- no-explicit-proxy client probe using `curl`, `wget`, or `nc`;
- Mac-side dnsmasq and mihomo logs after the client probe.

If the Android image lacks command-line probe tools, record that as an ADB
client-probe boundary. A future probe APK should stay thin and stable, with the
Mac-side logs and status remaining the authority for gateway behavior.

## Proxy egress proof

same-LAN proxy egress can be validated before imported subscriptions by using
the generated `upstream_proxy` slice:

- `OMG_SAME_LAN_UPSTREAM_PROXY_ENABLED=true`;
- `OMG_SAME_LAN_UPSTREAM_PROXY_TYPE=http` or `socks5`;
- `OMG_SAME_LAN_UPSTREAM_PROXY_SERVER=<lan-proxy-ip>`;
- `OMG_SAME_LAN_UPSTREAM_PROXY_PORT=<lan-proxy-port>`;
- `OMG_SAME_LAN_UPSTREAM_PROXY_MATCH_DOMAIN=api.ipify.org`.

This renders a single `open-surge-egress` select group and a single domain rule.
The proof requires Android route/DNS evidence, Mac-side fake-ip DNS evidence,
`mihomo.log` showing `Domain(api.ipify.org) using open-surge-egress[...]`, and a
client-visible exit IP from `https://api.ipify.org/` matching the upstream proxy
path.

The 2026-07-09 same-LAN run validated this narrower proxy-egress layer with a
Pixel test phone and a LAN HTTP proxy. The observed Android route was via the
Mac LAN IP, Android explicit proxy was unset, dnsmasq saw `api.ipify.org` from
the Android source IP, `mihomo.log` selected
`open-surge-egress[same-lan-http-egress]`, and the Android browser displayed the
same exit IP observed when the Mac used that LAN proxy directly.

This does not prove imported subscriptions or policy-group switching. For
policy switching, the generated group must contain at least two selectable
members, such as the LAN proxy and `DIRECT`, and the selected member should be
changed with `omg policy-select --config <path> --group <name> --policy <name>`
before repeating the same exit-IP probe.

## Imported provider policy switching

`make same-lan-start-tun-imported-egress` starts the same-LAN TUN config with an
imported profile rendered from `tests/lab/mihomo-profile.imported-tun-egress.yaml`.
It also starts a user-level local helper that serves the HTTP provider and a
controlled HTTP CONNECT proxy. The rendered profile contributes:

- `tun-egress-provider`;
- `TunEgress` with `DIRECT` and provider-backed `egress-proxy`;
- a domain rule for `OMG_SAME_LAN_TEST_HOST`;
- `MATCH,DIRECT` fallback.

`make same-lan-adb-check-imported-egress` is the evidence entrypoint. It keeps
the Android device on the no-explicit-proxy same-LAN path, checks that the live
policy/provider state contains `TunEgress` and `egress-proxy`, probes once while
`TunEgress[DIRECT]` is selected, switches to `egress-proxy` through
`omg policy-select`, then probes again. The required signals are `mihomo.log`
entries for both selected policies and the controlled proxy log observing
`CONNECT <host>:443` only after the switch.

This proves same-LAN transparent TUN policy switching to a controlled local
proxy. It does not prove a real subscription node, a real remote exit IP, or
whole-LAN rollout readiness.

### Manual Android evidence

The 2026-07-10 real-device run deliberately used no ADB control. An Android
test phone was manually configured with the Mac as its gateway and DNS and no
explicit proxy. A fresh browser request to `example.com:443` first logged
`TunEgress[DIRECT]` with no controlled-proxy entry. After selecting
`egress-proxy`, a second fresh request logged `TunEgress[egress-proxy]` and the
controlled proxy logged `CONNECT example.com:443`; both browser requests
loaded successfully. `make same-lan-stop` then removed the runtime state,
same-LAN PF anchor, forwarding, listeners, and helper.

### Imported egress runner contract

`runtime/same-lan/config-tun.yaml` is already in the imported profile's
directory, so its generated `mihomo.profile` must be
`./mihomo-profile.imported-tun-egress.yaml`, not a path prefixed with
`runtime/same-lan/`. The helper's readiness requires both the local HTTP
provider and controlled CONNECT proxy ports to accept connections; a provider
response alone is not sufficient evidence that the egress switch can work.

## Acceptance

The first same-LAN TUN acceptance requires:

- the test Android default gateway and DNS point at the Mac LAN IP;
- Android has no explicit proxy dependency;
- DNS to the Mac returns through OpenSurge/mihomo fake-ip handling;
- Android can trigger a connection to the test host;
- `mihomo.log` shows the target connection through the TUN path;
- stop restores PF, IPv4 forwarding, runtime state, and DNS listener state.

With `same-lan-start-tun-proxy`, the gate can additionally prove one-domain
remote proxy egress through a controlled LAN proxy. With
`same-lan-start-tun-imported-egress` and `same-lan-adb-check-imported-egress`,
it can prove imported provider-backed policy switching to a controlled local
proxy. It still does not prove IPv6, DoH/Private Relay, UDP/QUIC, global router
DHCP rollout, all-device compatibility, full subscription compatibility, or a
real remote exit IP.
