# mihomo profile overlay

When a task involves importing existing mihomo or Clash-style profiles, keep the
boundary clear: mihomo remains the proxy engine, but OpenSurge owns the Mac
gateway overlay.

## Modes

`mihomo.profile_mode: "managed"` is the default. OpenSurge renders its minimal
DIRECT/smoke mihomo config.

`mihomo.profile_mode: "imported"` reads `mihomo.profile` and imports only these
top-level mihomo engine sections. Relative `mihomo.profile` paths are resolved
from the OpenSurge config file's directory. Relative `path:` entries inside
imported `proxy-providers` and `rule-providers` are resolved from the imported
mihomo profile's directory. When starting or validating mihomo for an imported
profile, OpenSurge passes `-d <profile-dir>` so mihomo SAFE_PATHS accepts those
provider files:

- `proxies`
- `proxy-providers`
- `proxy-groups`
- `rule-providers`
- `rules`

## Gateway-owned fields

Imported profiles must not become raw pass-through configs. OpenSurge still
renders and owns:

- LAN binding through `mixed-port`, `allow-lan`, and `bind-address`;
- `external-controller`, so `status`, `doctor`, and policy-group CLI commands
  have the expected API target;
- `profile.store-selected: true`, so mihomo can persist selected policy-group
  members across restarts;
- DNS listener, fake-ip behavior, and TUN DNS hijack;
- TUN device, stack, routing flags, and LAN/private route exclusions.

This prevents a desktop mihomo profile from disabling LAN access, turning off
DNS/TUN, changing controller ports, or reintroducing unsupported transparent
proxy paths.

## Validation

Imported profile support is a mihomo config-generation change. Run `make test`
for code-level coverage. `doctor` includes a `mihomo config render` check so an
unreadable imported profile or missing `rules` section fails before gateway
startup. Use `go run ./cmd/omg render-mihomo --config <path>` to inspect the
final overlaid mihomo config without root or service startup. Use
`go run ./cmd/omg validate-mihomo --config <path>` for a stronger non-root check
that renders the final config and runs mihomo's own `-t` validation with the same
`-d` directory OpenSurge uses at startup. This command requires `mihomo.binary`
in the OpenSurge config to point to an installed mihomo binary.

When mihomo is running, use `omg policies --config <path>` to list policy groups,
`omg policy-select --config <path> --group <name> --policy <name>` to switch the
selected member, and `omg connections --config <path>` to inspect active mihomo
connections. Use `omg providers --config <path>` to inspect proxy/rule
providers, and `omg provider-update --config <path> --provider <name>` to
refresh one proxy provider. `policy-select` first reads live groups and rejects
unknown group or policy names before sending the selection change. These are
control-plane checks. `make policy-control-test` also proves one local
mixed-port request can be switched from `DIRECT` to a controlled HTTP CONNECT
proxy with `policy-select`, and verifies both file and locally served HTTP
proxy-provider refresh. It still does not require real-device validation unless
the change also touches gateway, DNS, TUN, or traffic-capture behavior.

If a change affects generated runtime traffic defaults, TUN behavior, DNS
behavior, or real proxy egress semantics, use the matching network gate:
`make lab-test`, `make lab-test-tun`, `make lab-test-tun-imported-profile`,
`make lab-test-tun-imported-egress`, or a documented real-device smoke.

`make lab-test-tun-imported-profile` runs the TUN gate with
`tests/lab/mihomo-profile.imported-tun.yaml`, which keeps rules at
`MATCH,DIRECT`. It proves the imported profile overlay can start in the TUN lab;
it does not prove an external proxy egress.

`make lab-test-tun-imported-egress` runs the TUN gate with
`tests/lab/mihomo-profile.imported-tun-egress.yaml`. The fixture uses a local
HTTP provider to add `egress-proxy`, then the lab switches `TunEgress` from
`DIRECT` to `egress-proxy` through `omg policy-select`. The direct signals are
`mihomo.log` entries for `TunEgress[DIRECT]` and `TunEgress[egress-proxy]`, plus
the controlled proxy observing `CONNECT <host>:443` only after the switch. This
proves controlled local proxy egress switching through transparent TUN; it does
not prove a real subscription node, remote exit IP, same-LAN, or real-device
behavior.
