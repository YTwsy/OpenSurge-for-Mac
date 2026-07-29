# Local Mac routing modes

OpenSurge provides a **Rule / Global / Direct** switch similar to Clash Verge
Rev, but its scope is explicitly limited to the gateway Mac. It does not change
mihomo's top-level `mode: rule`, downstream-device rules, device selectors, or
DHCP/DNS configuration.

## Modes

| Mode | New connections from the Mac | Downstream devices |
| --- | --- | --- |
| Rule | Continue through the imported/managed gateway rules | Continue through gateway rules or their device policy |
| Global | Send TCP through the dedicated local-global selector | Unchanged |
| Direct | Use `DIRECT` | Unchanged |

“Global” does not rewrite the global policy for every device. It only sends
connections with the local-Mac identity to a dedicated hidden selector. If the
selected egress does not support UDP, or OpenSurge cannot confirm that support,
local UDP is sent to `REJECT` instead of silently falling through to gateway
rules or direct access.

Loopback, LAN/private, link-local, CGNAT, and multicast destinations remain
direct before the mode rules. This preserves local management access even when
a remote global egress is selected.

## Local identity and downstream isolation

OpenSurge constrains both the **inbound type** and the **source address**:

- TUN connections whose mihomo local-TUN source identity is `198.18.0.1`;
- explicit mixed-port connections from `127.0.0.0/8` or the gateway Mac's LAN
  IPv4 address.

Downstream connections carry their own LAN IPv4 source and cannot match those
local rules. They continue into device `SRC-IP-CIDR` overrides and the
imported/managed gateway rules.

The Web GUI's local-Mac mode and a device's “Follow gateway rules / Dedicated
device egress” are therefore orthogonal controls:

- changing the local mode is live and affects new connections;
- changing device identity, routing mode, or rules still requires save and
  reload;
- changing an applied device selector remains scoped to that device.

## TUN and the macOS system proxy

OpenSurge does not enable or rewrite the macOS **System Settings → Network →
Proxies** configuration. With TUN enabled, routable local IPv4 traffic entering
the TUN uses this mode. Applications that explicitly use the OpenSurge
mixed-port enter the same mode.
The Web GUI connectivity probe originates from the local Control Service
through that mixed-port, so it also reflects the current local mode. It is not
evidence for a downstream device's gateway-rule path.

“Global” here therefore means a global choice for local-Mac traffic that enters
the OpenSurge data plane. It is not unconditional control of every protocol,
Network Extension, or downstream device. Existing connections are not forcibly
terminated; start a new connection when checking a mode change.

## CLI

```bash
./bin/omg local-routing --config /etc/open-mihomo-gateway/config.yaml

./bin/omg local-routing-set \
  --config /etc/open-mihomo-gateway/config.yaml \
  --mode rule

./bin/omg local-routing-set \
  --config /etc/open-mihomo-gateway/config.yaml \
  --mode global \
  --policy "Proxy"

./bin/omg local-routing-set \
  --config /etc/open-mihomo-gateway/config.yaml \
  --mode direct
```

Internal selectors use the reserved `open-surge/mac-*` namespace. Mihomo
persists their selections through `profile.store-selected: true`. Generic
policies, providers, proxy health, and `policy-select` do not expose or accept
these internal groups; use the dedicated `local-routing` commands or the Web
GUI.
