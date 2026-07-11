# same-WiFi two-device policy gate

[简体中文](SAME-WIFI-DEVICE-POLICY.zh-CN.md) | English

This real-device gate covers exact DHCP reservations, two independent device
selectors, a device rule slot, UDP fail-closed behavior, and full recovery in
`same_wifi_dhcp`. It remains **Experimental / cooperative IPv4**: manual router
routes and IPv6 can bypass the Mac on a shared Wi-Fi.

Complete the [recovery reference](WIFI-DHCP-RECOVERY.md) first. Disable MAC
randomization for this SSID, record each client's actual Wi-Fi MAC, keep a
static recovery device outside the DHCP pool, and disconnect both test clients
before startup so stale leases cannot occupy their reserved addresses.

Set the two `OMG_SAME_WIFI_DEVICE_*` MAC, IP, and ADB serial triples, the DHCP
pool/protected addresses, and the protected LAN HTTP proxy described by the
Chinese runbook. Then run:

```sh
make same-wifi-dhcp-start-device-policy
make same-wifi-dhcp-adb-check-device-policy
make same-wifi-dhcp-stop
```

After manually restoring router DHCP and returning both clients to automatic
IP/DNS, set `OMG_SAME_WIFI_DHCP_ROUTER_DHCP_RESTORED=confirmed` and
`OMG_SAME_WIFI_DHCP_CLIENTS_AUTOMATIC=confirmed`, then run:

```sh
make same-wifi-dhcp-verify-device-policy-recovery
```

The startup gate rejects any competing DHCP OFFER. The recovery gate requires
a restored OFFER before returning the Mac to DHCP, then checks the Mac lease,
server/default route and both clients' router path and HTTPS. Do not claim this
gate passed unless all commands and manual router steps were actually completed.
