---
title: Per-device policies are compiled into one mihomo instance
kind: decision
status: active
---

# Per-device policies are compiled into one mihomo instance

OpenSurge for Mac does not start a separate proxy engine or carry a full
mihomo profile for every LAN device. A device-policy JSON file records stable
device identity as MAC plus a reserved IPv4 lease, then compiles a device's
rules into one shared mihomo configuration.

Each registered device receives a `device/<id>/default` selector. A rule with
its own `policies` receives a separate `device/<id>/<rule-id>` selector. The
compiled mihomo rules use IPv4 `SRC-IP-CIDR` so a selection changes only the
specified device. `devices` reports configured identities and lease state;
`device-policy-select` only accepts selectors belonging to the named device.

Rules may combine domain, IP CIDR, TCP/UDP protocol, port, and rule-provider
conditions. Populated condition types are ANDed, while values inside one type
are ORed into separate mihomo rules. Small lists use inline rule-providers;
large shared lists may use HTTP providers. HTTP MRS is valid only for domain
and ipcidr behavior. OpenSurge ships no household templates or third-party
rule content; templates and rule-set URLs are operator-owned data.

Order is part of the contract: device overrides precede global rules, device
defaults follow global rules, and an imported profile's terminal `MATCH` stays
last. Reject an imported profile with rules after `MATCH`, rather than silently
making device defaults unreachable.

The supported identity boundary is MAC-backed IPv4 DHCP reservations plus
`SRC-IP-CIDR`. It is not IPv6 identity or MAC matching performed inside mihomo.
Registered addresses must stay in the gateway `/24` and cannot be its network,
broadcast, or gateway address.

Use `make test` for the compiler, JSON validation, template, and rule-provider
coverage. Use `make lab-test-tun-device-policy` for data-path changes to
reservations, independent per-device selectors, or device overrides. That Lab
gate proves two VM clients receive `.101` and `.102`, choose different TUN
egress paths without affecting each other, and enforce a device-specific
domain `REJECT`.
