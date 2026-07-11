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

If you plan to disable router DHCP on a dedicated test Wi-Fi and let OpenSurge
serve DHCP/DNS on that same Wi-Fi, read the
[same-WiFi DHCP recovery reference](WIFI-DHCP-RECOVERY.md) first. Recovery is
part of the acceptance criteria.

For the separate full DHCP/TUN/provider/policy/egress gate, use the
[same-WiFi DHCP imported egress runner](WIFI-DHCP-RUNNER.md). It uses
`same_wifi_dhcp`; this page's `same_lan` runner remains DHCP-disabled.

For two exact reservations and independent per-device selectors, use the
[same-WiFi two-device policy gate](SAME-WIFI-DEVICE-POLICY.md). It adds active
competing-DHCP detection and a separate recovery gate; until that real run
passes, the capability remains Experimental / cooperative IPv4.

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
with `omg policy-select --config <path> --group open-surge-egress --policy <member>`
and repeat the `api.ipify.org` probe.

## Imported Policy Egress Smoke

To exercise a path closer to imported subscriptions while still keeping the
proxy endpoint controlled and local, start the same-LAN imported egress fixture:

```sh
make same-lan-start-tun-imported-egress
```

This writes `runtime/same-lan/config-tun.yaml` with
`mihomo.profile_mode: "imported"` and renders
`runtime/same-lan/mihomo-profile.imported-tun-egress.yaml` from the lab fixture.
Because imported profile paths are resolved from the config file's directory,
the generated config references this profile as
`./mihomo-profile.imported-tun-egress.yaml`.
The rendered profile contains:

- an HTTP `proxy-provider` named `tun-egress-provider`;
- a `TunEgress` select group containing `DIRECT` and provider-backed
  `egress-proxy`;
- a domain rule for `OMG_SAME_LAN_TEST_HOST`, defaulting to `example.com`;
- `MATCH,DIRECT` for all other traffic.

The runner also starts a user-level local helper that serves the HTTP provider
and listens as a controlled HTTP CONNECT proxy on `127.0.0.1`. It reports
ready only after both the provider and proxy ports accept connections; both
must remain alive through the two client probes.

After the Android test phone points gateway and DNS at the Mac LAN IP, run:

```sh
make same-lan-adb-check-imported-egress
```

That ADB check verifies the Android route/DNS/no-explicit-proxy preflight,
checks that `TunEgress` contains `egress-proxy`, sends one no-explicit-proxy
HTTPS probe while `TunEgress[DIRECT]` is selected, switches with
`omg policy-select --group TunEgress --policy egress-proxy`, then repeats the
probe. It requires `mihomo.log` to show both `TunEgress[DIRECT]` and
`TunEgress[egress-proxy]`, and requires the controlled proxy log to observe
`CONNECT <host>:443` only after the switch.

This smoke proves same-LAN transparent TUN policy switching to a controlled
local proxy. It does not prove a real subscription node or remote exit IP.

### Manual Phone Check (without ADB)

When Android must be operated manually, do not run the ADB target. Instead:

1. On the test phone, use an unused static IPv4 address on the same Wi-Fi
   subnet, point both its gateway and DNS at the Mac LAN IP, and keep its
   explicit proxy disabled.
2. Start the fixture, select `DIRECT`, clear the controlled proxy log, then
   access `https://example.com/` in a fresh private browser tab.
3. Confirm `mihomo.log` records
   `example.com:443 ... using TunEgress[DIRECT]` and that the controlled proxy
   log is empty.
4. Select `egress-proxy` with `omg policy-select`, then repeat the browser
   access in a fresh private tab. Confirm `mihomo.log` records
   `TunEgress[egress-proxy]` and the controlled proxy log records
   `CONNECT example.com:443`.

The 2026-07-10 manual Android run completed both browser probes successfully
without ADB and completed `make same-lan-stop` cleanup. It remains evidence for
the controlled local proxy only, not a real subscription node or remote exit.

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

If router DHCP was disabled during the test, follow the
[same-WiFi DHCP recovery reference](WIFI-DHCP-RECOVERY.md) to restore router
DHCP, Mac Wi-Fi DHCP, and automatic addressing on test clients.

## Current Boundary

This smoke only proves that one Android test device on the same LAN can point
its default gateway and DNS at the Mac, then send no-explicit-proxy traffic into
the OpenSurge TUN path. With `same-lan-start-tun-proxy`, it can also prove one
domain's real upstream proxy egress. With
`same-lan-start-tun-imported-egress` plus `same-lan-adb-check-imported-egress`,
it can prove imported provider-backed policy switching to a controlled local
proxy. It does not prove global router DHCP rollout, all-device compatibility,
IPv6, DoH/Private Relay, UDP/QUIC, full subscription compatibility, or a real
remote exit IP.

Router-DHCP takeover is not an implied capability of these narrow targets. Use
the `same-wifi-dhcp-*` full runner and its recovery contract instead.
