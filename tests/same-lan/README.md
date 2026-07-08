# same-LAN TUN Smoke Test

[简体中文](README.zh-CN.md) | English

This guide covers the same-LAN TUN default-gateway smoke. Unlike the isolated
downstream LAN under `tests/real-device/`, the Mac and the Android test device
stay on the same home or office LAN. OpenSurge does not take over DHCP on the
main LAN.

## Topology

```text
Home router / main Wi-Fi: 192.168.1.1
        |
        +-- Mac en0: 192.168.1.20
        |     OpenSurge same_lan + mihomo TUN + DNS-only dnsmasq
        |
        +-- Android phone: 192.168.1.x
              default gateway: 192.168.1.20
              DNS: 192.168.1.20
```

The first acceptance slice targets one test phone. Do not run OpenSurge DHCP on
the main LAN, and do not globally change the router's DHCP options to the Mac
unless you are ready to affect every device on that LAN.

## Start

The runner infers the interface from the macOS default route and reads the Mac
IPv4 address from that interface:

```sh
make same-lan-start-tun
```

Override both values when needed:

```sh
OMG_SAME_LAN_IFACE=en0 \
OMG_SAME_LAN_MAC_IP=192.168.1.20 \
make same-lan-start-tun
```

The runner writes `runtime/same-lan/config-tun.yaml` with:

- `gateway.mode: "same_lan"`;
- matching `gateway.interface` and `gateway.upstream_interface`;
- `dhcp.enabled: false`;
- `dns.listen` bound to the Mac's main LAN IP;
- `dns.upstream: "127.0.0.1#1053"` for mihomo fake-ip DNS;
- `transparent.mode: "tun"`.

## Android ADB Check

First point the Android test phone's Wi-Fi default gateway and DNS at the Mac's
LAN IP. The runner does not try to persistently edit Wi-Fi settings; it uses ADB
to collect machine-readable validation results.

```sh
make same-lan-adb-check
```

Specify a serial when multiple devices are connected:

```sh
OMG_SAME_LAN_ADB_SERIAL=57081FDCQ008KZ make same-lan-adb-check
```

The ADB check verifies:

- the device is authorized and listed as `device`;
- Android's default route includes `via <mac-lan-ip>`;
- Android can query `@<mac-lan-ip> example.com` with `nslookup` or `dig`
  when the image provides one of those tools;
- Android global explicit proxy state is not the reason the test passes;
- Android can issue a no-explicit-proxy probe with `curl`, `wget`, or `nc`;
- Mac-side `dnsmasq.log` and `mihomo.log` show the DNS/fake-ip and TUN signals.

If the Android image lacks `nslookup` and `dig`, the ADB gate continues with the
TCP probe and infers DNS from the Mac-side `dnsmasq.log` entry for the Android
source IP. If the image also lacks `curl`, `wget`, and `nc`, this gate stops at
the missing client-probe boundary. A later probe APK should stay thin and
stable; gateway truth should remain in Mac-side logs and status.

## Proxy Egress Smoke

The first proxy-egress check should use a small generated mihomo rule instead
of a full subscription. Point `upstream_proxy` at a known LAN proxy and match one
diagnostic domain:

```sh
OMG_SAME_LAN_TEST_HOST=api.ipify.org \
OMG_SAME_LAN_UPSTREAM_PROXY_NAME=same-lan-http-egress \
OMG_SAME_LAN_UPSTREAM_PROXY_TYPE=http \
OMG_SAME_LAN_UPSTREAM_PROXY_SERVER=192.168.1.101 \
OMG_SAME_LAN_UPSTREAM_PROXY_PORT=8080 \
OMG_SAME_LAN_UPSTREAM_PROXY_MATCH_DOMAIN=api.ipify.org \
make same-lan-start-tun-proxy

OMG_SAME_LAN_TEST_HOST=api.ipify.org make same-lan-adb-check
```

For a SOCKS5 LAN proxy, use `OMG_SAME_LAN_UPSTREAM_PROXY_TYPE=socks5` with the
SOCKS server and port. The generated mihomo config creates one
`open-surge-egress` select group and one rule:

```yaml
rules:
  - DOMAIN,api.ipify.org,open-surge-egress
  - MATCH,DIRECT
```

This proves real proxy egress only when the evidence lines up:

- Android default gateway and DNS still point at the Mac LAN IP;
- Android global explicit proxy remains unset;
- `dnsmasq.log` shows `api.ipify.org` from the Android source IP;
- `mihomo.log` shows `Domain(api.ipify.org) using open-surge-egress[...]`;
- the Android browser page for `https://api.ipify.org/` shows the expected
  upstream proxy exit IP.

The 2026-07-09 same-LAN run validated this with a Pixel test phone,
`api.ipify.org`, and a LAN HTTP proxy. The Android page and Mac-side direct
proxy check both reported the same exit IP, while `mihomo.log` showed
`open-surge-egress[same-lan-http-egress]`.

Policy-group switching is a separate smoke. The current generated group has one
proxy member, so it can prove "matched proxy" but not meaningful selection
changes. To validate switching, add at least two group candidates, for example
the LAN proxy and `DIRECT`, then switch the selected `open-surge-egress` member
through the mihomo API and repeat the `api.ipify.org` probe.

## Stop

```sh
make same-lan-stop
```

Expected cleanup:

- OpenSurge runtime state is removed;
- the same-LAN PF anchor is unloaded;
- IPv4 forwarding is restored to the pre-start value;
- the DNS listener no longer occupies port 53 on the Mac LAN IP;
- the Mac and main LAN return to the pre-start normal network state.

## Current Boundary

This smoke only proves that one Android test device on the same LAN can point
its default gateway and DNS at the Mac, then send no-explicit-proxy traffic into
the OpenSurge TUN path. With `same-lan-start-tun-proxy`, it can also prove one
domain's real upstream proxy egress. It does not prove global router DHCP
rollout, all-device compatibility, IPv6, DoH/Private Relay, UDP/QUIC, imported
subscriptions, or policy-group switching.
