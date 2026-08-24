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
- on the `DEFAULT-TUN` inbound, the exact fake-IPv6 local identity
  `fdfe:dcba:9876::1/128` when AAAA is enabled but IPv6 TUN is inactive, or the
  exact host-TUN identity `fdfe:dcba:9877::1/128` when IPv6 TUN is effective;
- explicit mixed-port connections from `127.0.0.0/8` or the gateway Mac's LAN
  IPv4 address.

Downstream IPv4 connections carry their own LAN source. Downstream IPv6 enters
through the separate `opensurge-ipv6` packet listener with a device `IN-USER`
and a source from `fdfe:dcba:9878::/64`. Neither path can match the local rules;
they continue into device overrides and the imported/managed gateway rules.

### AAAA and local modes

`dns.ipv6` and `transparent.tun_ipv6` are independent. Even when downstream
IPv6 is set to `auto` and remains inactive because no native upstream IPv6 is
available, enabling AAAA responses still makes mihomo DNS return fake IPv6
addresses from `fdfe:dcba:9876::/64`. Some applications prefer those answers.
When IPv6 TUN is inactive, the corresponding `fdfe:dcba:9876::1/128`
system-TUN source identity is sent through `open-surge/mac-mode-*`, so AAAA
does not need to be disabled merely to make local Direct mode effective. When
IPv6 TUN is effective, mihomo's explicit `fdfe:dcba:9877::1/128` host-TUN
identity replaces it and uses the same mode.

Both IPv6 alternatives additionally require `IN-NAME,DEFAULT-TUN`; only the
effective identity is generated. They do not match the whole fake-IP `/64`,
downstream `fdfe:dcba:9878::/64`, or the
`opensurge-ipv6` listener. Reload the gateway after upgrading to a version with
this support so it generates the new rules. Mode switches still affect only
new connections; refresh or let applications recreate existing connections.

The Web GUI's local-Mac mode and a device's “Follow gateway rules / Dedicated
device egress” are therefore orthogonal controls:

- changing the local mode is live and affects new connections;
- changing device identity, routing mode, or rules still requires save and
  reload;
- changing an applied device selector remains scoped to that device.

## TUN and the macOS system proxy

The local-Mac Rule / Global / Direct switch does not itself enable or rewrite
macOS **System Settings → Network → Proxies**. With TUN enabled, routable local
IPv4 traffic and the system-TUN IPv6 identities listed above use this mode.
Applications that explicitly use the OpenSurge mixed-port enter the same mode.

Network Settings has a separate, off-by-default **local system-proxy
coordination** compatibility option. In TUN mode it temporarily points the
current upstream network service's HTTP and HTTPS proxies at
`127.0.0.1:<mixed-port>`. This covers known TUN-only local-DNS conflicts caused
by SafeDNS, DNS Proxy, content filters, or other Network Extensions. The
pre-start settings are persisted in runtime state and restored before stop,
startup rollback, or after a failed mihomo restart. Existing HTTP/HTTPS proxy,
PAC, auto-discovery, or authenticated proxy settings make startup fail closed.
The option affects only Mac apps that honor system proxy settings; it does not
own SOCKS, PAC, bypass lists, replace TUN, or alter downstream devices.

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
