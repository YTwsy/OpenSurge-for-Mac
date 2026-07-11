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
broadcast, gateway, or declared protected address. same-Wi-Fi DHCP start also
rejects a reservation when ARP observes a different MAC at that IPv4; no ARP
reply is only an inconclusive signal, not proof of vacancy.

The configured policy is desired state. One start compiles it exactly once into
an immutable bundle, validates the final mihomo configuration before forwarding
is enabled, and saves the bundle plus digest as runtime applied state. Running
`devices` and `device-policy-select` consume that applied snapshot; a changed
or invalid desired file is surfaced as drift/error rather than reinterpreting a
running gateway. Policy updates are stop/start only in this MVP.

Mihomo may continue rule evaluation for UDP when a selected outbound lacks UDP
support. Generated selector/default rules therefore add a same-condition
`REJECT` fallback by default; `on_unsupported: fallthrough` is an explicit
opt-out. Imported profiles are parsed as YAML for namespace/reference checks:
generated `device/` groups and `open-surge-ruleset-` providers are reserved,
and candidates/actions must reference known targets or explicit built-ins.

Use `make test` for the compiler, JSON validation, template, and rule-provider
coverage. Use `make lab-test-tun-device-policy` for data-path changes to
reservations, independent per-device selectors, or device overrides. That Lab
gate proves two VM clients receive `.101` and `.102`, choose different TUN
egress paths without affecting each other, enforce a device-specific domain
`REJECT`, preserve exact applied DHCP identities, expose desired/applied drift,
and fail close UDP/443 through an HTTP-only selected outbound.
