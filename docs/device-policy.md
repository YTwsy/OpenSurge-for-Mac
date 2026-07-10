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
