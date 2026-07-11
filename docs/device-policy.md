# Per-device policy overlays

OpenSurge runs one mihomo process. Device policy does not create a mihomo
process or a complete profile per client. Instead, OpenSurge assigns a stable
IPv4 lease to each registered MAC address, generates an independent selector
group for every device, and routes traffic with mihomo `SRC-IP-CIDR` rules.

This feature is optional. Point `device_policy.file` at a JSON document; the
empty [starter document](../examples/device-policy.example.json) is valid but
does not enable any device policy.

```yaml
device_policy:
  file: "./devices.json"
```

The device-policy file is resolved relative to the gateway configuration file.
All registered IPv4 addresses must be unique, must remain in the gateway `/24`,
and must not be the network, broadcast, or `gateway.lan_ip` address.

For `same_wifi_dhcp`, declare every router, recovery client, LAN proxy, or
other static address that must never become a reservation:

```yaml
device_policy:
  file: "./devices.json"
  protected_ipv4: "192.168.1.1,192.168.1.21,192.168.1.101"
```

Reservations may be inside the dynamic DHCP range; `devices --format json`
marks that relation explicitly. A reservation may not equal a protected
address. At same-Wi-Fi DHCP start, OpenSurge also warms ARP and refuses a
reservation when a different MAC is already observed. No ARP reply is not a
guarantee that an address is vacant, so router-DHCP isolation and recovery
evidence remain required.

## Model

There are no built-in household, parental-control, streaming, or vendor rule
lists. Operators own the policy content. The JSON model has four independent
collections:

- `devices`: stable identity (`id`, MAC, reserved IPv4, profile id);
- `profiles`: default selector candidates plus device rule overlays;
- `templates`: optional reusable profile defaults and rule fragments;
- `rule_sets`: inline or HTTP mihomo rule-provider definitions.

The following is a syntax example only. `Proxy` must already exist in the
managed or imported global mihomo profile.

```json
{
  "templates": [
    {
      "id": "baseline",
      "default_policies": ["DIRECT", "Proxy"]
    }
  ],
  "rule_sets": [
    {
      "id": "media",
      "behavior": "domain",
      "payload": ["media.example"]
    }
  ],
  "profiles": [
    {
      "id": "phone",
      "template": "baseline",
      "rules": [
        {
          "id": "block-udp",
          "match": {"protocols": ["udp"]},
          "action": "REJECT"
        },
        {
          "id": "media",
          "match": {"rule_sets": ["media"]},
          "policies": ["Proxy", "DIRECT"]
        }
      ]
    }
  ],
  "devices": [
    {
      "id": "alice-phone",
      "mac": "aa:bb:cc:dd:ee:01",
      "ipv4": "192.168.50.101",
      "profile": "phone"
    }
  ]
}
```

`default_policies` creates `device/<device-id>/default`. A rule with
`policies` creates a separately selectable group named
`device/<device-id>/<rule-id>`. A rule with `action` routes directly to a
built-in policy such as `DIRECT` or `REJECT`, or to an existing global mihomo
group.

Policy candidates and actions are checked against the imported profile's
proxy/group namespace before start. `DIRECT`, `REJECT`, `REJECT-DROP`, and
`REJECT-TINYGIF` are the explicit built-ins. OpenSurge reserves `device/` for
generated groups and `open-surge-ruleset-` for generated rule providers, so an
imported profile may not occupy those namespaces.

## Matching and precedence

`domains`, `ip_cidrs`, `protocols` (`tcp` or `udp`), `ports`, and `rule_sets`
can be combined. Different populated fields are ANDed; entries within one field
are ORed and compile to separate mihomo rules. For example, a domain and a
protocol compile to:

```text
AND,((SRC-IP-CIDR,192.168.50.101/32),(DOMAIN-SUFFIX,media.example),(NETWORK,tcp)),device/alice-phone/media
```

Generated ordering is deliberate:

1. device-specific overrides;
2. imported or managed global rules;
3. per-device default selector;
4. global terminal `MATCH`.

An imported profile must keep `MATCH` terminal. OpenSurge rejects an imported
profile that places later rules after a terminal `MATCH`, because the device
default could never be reached safely.

## UDP unsupported-outbound behavior

Mihomo continues downward when UDP selects an outbound that does not support
UDP. Device selectors therefore compile as fail-closed by default: every
selector/default rule is immediately followed by the same condition with
`REJECT`. This prevents QUIC or other UDP traffic from silently reaching a
later global rule or `MATCH,DIRECT`.

Set `on_unsupported: "fallthrough"` on a profile, template, or individual
rule only when a later rule is intentionally responsible for that fallback.
The default is `"reject"`. A proxy/group name being present does not prove UDP
capability; provider-backed candidates require live traffic evidence.

## Large rule sets and templates

`rule_sets` support `inline` and `http` providers with `domain`, `ipcidr`, or
`classical` behavior. HTTP providers may use `yaml`, `text`, or `mrs`; mihomo
MRS is accepted only for `domain` and `ipcidr` behavior. Use an HTTP MRS set for
large shared domain/IP lists, and use profile templates to reuse policy choices
without cloning a full mihomo profile.

## Operations

```sh
./bin/omg devices --config ./config.yaml --format json

./bin/omg device-policy-select \
  --config ./config.yaml \
  --device alice-phone \
  --slot default \
  --policy Proxy
```

The second command changes only the named device selector. It does not switch
another device's selector or the global policy group.

## Desired and applied policy

`start` compiles the policy once, renders DHCP and mihomo from that same
immutable bundle, validates mihomo before forwarding is enabled, and writes
`runtime/device-policy.applied.json` plus its digest in `runtime/state.json`.
`devices` and `device-policy-select` use that applied snapshot while the
gateway is running. `devices` compares the current desired digest and reports
`drift`; an invalid desired file is returned as `desired_error` without hiding
the running applied policy.

Editing `devices.json` does not reload the gateway. For this MVP, use `stop`,
update the file, then `start`. Stale lease rows for a managed MAC with the old
reserved IPv4 are removed at startup; wait for a fresh DHCP ACK before testing
policy traffic.

`lease_active` means only that dnsmasq has an unexpired lease. It is not a
reachability claim. `policy_identity_ready` is true only when the gateway is
using an applied policy and the lease MAC, IPv4, and expiry all match the
applied reservation.

## Validation boundary

The feature currently identifies LAN devices through MAC-backed IPv4 DHCP
reservations and emits IPv4 `SRC-IP-CIDR` rules. It does not provide IPv6 device
identity, MAC matching inside mihomo, or curated third-party rule content.

The required data-plane gate is:

```sh
make lab-up
sudo -v
make lab-test-tun-device-policy
make lab-down
```

It uses two Lima clients, verifies the fixed `.101` and `.102` leases, distinct
TUN policy groups and egress paths, independent selector changes, and a
device-specific domain `REJECT`. Rule/template/provider compilation is covered
by unit tests and does not require a Lab run for each operator-defined rule.
