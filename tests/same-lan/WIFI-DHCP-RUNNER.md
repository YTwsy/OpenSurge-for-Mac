# same-WiFi DHCP Imported Egress Full Runner

[简体中文](WIFI-DHCP-RUNNER.zh-CN.md) | English

This is a high-risk real-Wi-Fi gate. The Mac and Android client remain on one
Wi-Fi, the operator manually disables router DHCP, and OpenSurge takes over
DHCP/DNS on that Wi-Fi before proving TUN traffic and an imported,
provider-backed `TunEgress` switch.

It uses the explicit `gateway.mode: "same_wifi_dhcp"`. The existing
`same_lan` mode remains DHCP-disabled and must not be repurposed for this test.

## Preconditions

- Use a dedicated test SSID. Router DHCP and OpenSurge DHCP must never run
  together.
- Keep the Mac on a manually configured Wi-Fi IPv4 address and verify a second,
  static-address recovery device can open the router administration page.
- List every protected static address outside the new lease pool. For example,
  with Mac `.20`, recovery client `.21`, and LAN proxy `.101`, use `.120-.199`.
- Prepare an HTTP proxy on a protected LAN address, for example
  `192.168.1.101:8080`. The controlled local CONNECT proxy uses it as its next
  hop so its own upstream dial cannot re-enter TUN.
- Set the Android test client to DHCP/automatic addressing and no explicit HTTP
  proxy. For the automated gate, it must be authorized in ADB; a browser-only
  evidence path is available when ADB is intentionally unavailable.
- Follow the [same-WiFi DHCP recovery reference](WIFI-DHCP-RECOVERY.md) first.

The runner never changes router settings. The required
`OMG_SAME_WIFI_DHCP_ROUTER_DHCP_DISABLED=confirmed` value is an operator
receipt, not a remote router-state check.

## Run

After manually disabling router DHCP and confirming that the statically
addressed Mac still reaches the router:

```sh
OMG_SAME_WIFI_DHCP_ROUTER_DHCP_DISABLED=confirmed \
OMG_SAME_WIFI_DHCP_PROTECTED_IPS=192.168.1.101 \
OMG_SAME_WIFI_DHCP_EGRESS_UPSTREAM_HTTP_PROXY=192.168.1.101:8080 \
make same-wifi-dhcp-start-imported-egress
```

The default pool is `.120-.199` in the Mac's `/24`. Override it with
`OMG_SAME_WIFI_DHCP_RANGE_START` and `OMG_SAME_WIFI_DHCP_RANGE_END` when needed.
Reconnect the Android device in DHCP mode, then run the automated ADB gate:

```sh
make same-wifi-dhcp-adb-check-imported-egress
make same-wifi-dhcp-stop
```

Run stop before re-enabling router DHCP. Then re-enable router DHCP and return
the Mac and test client to automatic addressing.

### Manual phone evidence without ADB

When the Android test device must be operated manually, use its browser with no
explicit proxy and collect the same host-side evidence. After recording the
DHCP lease and DNS query, select each policy on the Mac, open
`https://example.com/` in the phone browser, and inspect the new log entries:

```sh
./bin/omg policy-select \
  --config runtime/same-wifi-dhcp/config-tun.yaml \
  --group TunEgress --policy DIRECT --format json
# Browse https://example.com/ on Android; expect TunEgress[DIRECT].

./bin/omg policy-select \
  --config runtime/same-wifi-dhcp/config-tun.yaml \
  --group TunEgress --policy egress-proxy --format json
# Browse https://example.com/ again; expect TunEgress[egress-proxy] and CONNECT.

tail -n 120 runtime/same-wifi-dhcp/logs/dnsmasq.log
tail -n 120 runtime/same-wifi-dhcp/logs/mihomo.log
tail -n 120 runtime/same-wifi-dhcp/egress/proxy.log
```

This is operator-recorded evidence rather than a replacement for the automated
ADB gate. Run `make same-wifi-dhcp-stop` as soon as the probes are complete.

## Evidence and boundary

The ADB gate requires the Android lease and DHCPACK in OpenSurge evidence, a
route and DNS path through the Mac without an explicit proxy, live provider
state plus `provider-update`, TUN traffic through `TunEgress[DIRECT]`, and a
post-`policy-select` request through `TunEgress[egress-proxy]` with a controlled
CONNECT log. The manual path records the applicable lease, DNS, policy, browser,
TUN, and CONNECT evidence from the phone and host logs. Stop verifies runtime
state, PF anchor, listeners, the local helper, and IPv4-forwarding restoration.

It does not prove broad device compatibility, IPv6, DoH/Private Relay, UDP/QUIC,
full subscription compatibility, or a real remote exit IP.
