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

## Runner

The runner is `tests/same-lan/smoke.sh` with Makefile entrypoints:

- `make same-lan-start-tun`;
- `make same-lan-start-tun-proxy`;
- `make same-lan-adb-check`;
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
changed through the mihomo API before repeating the same exit-IP probe.

## Acceptance

The first same-LAN TUN acceptance requires:

- the test Android default gateway and DNS point at the Mac LAN IP;
- Android has no explicit proxy dependency;
- DNS to the Mac returns through OpenSurge/mihomo fake-ip handling;
- Android can trigger a connection to the test host;
- `mihomo.log` shows the target connection through the TUN path;
- stop restores PF, IPv4 forwarding, runtime state, and DNS listener state.

With `same-lan-start-tun-proxy`, the gate can additionally prove one-domain
remote proxy egress through a controlled LAN proxy. It still does not prove IPv6,
DoH/Private Relay, UDP/QUIC, global router DHCP rollout, all-device
compatibility, imported profiles, or policy-group switching.
