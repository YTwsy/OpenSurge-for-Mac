# OpenSurge for Mac

[简体中文](README.zh-CN.md) | English

OpenSurge for Mac is a macOS CLI gateway MVP. It prepares a Mac to act as an
IPv4 LAN gateway, runs dnsmasq for DHCP/DNS, runs mihomo for proxying, and uses
macOS pf for NAT. Its goal is to turn a Mac into a
controlled IPv4 LAN gateway that can share proxy-backed connectivity with
phones, tablets, VMs, test devices, and other downstream clients.

The project direction is broader: a Mac-native, auditable gateway with
transparent routing, reproducible lab validation, and eventually a friendlier
control surface.

## Current scope

The current implementation is a CLI-driven MVP:

1. CLI, config, runtime state, and text/JSON status/doctor/logs/snapshot
   commands.
2. dnsmasq config, process management, and lease parsing.
3. mihomo config, process management, version API checks, and policy-group
   selection through the mihomo external-controller API.
4. pf anchor management and IPv4 forwarding restore.

Today OpenSurge for Mac can:

- prepare and inspect a gateway config from the CLI;
- start and stop DHCP/DNS, mihomo, pf NAT, and IPv4 forwarding with rollback;
- support explicit proxy traffic through mihomo `mixed-port`;
- support transparent proxying through mihomo TUN on macOS;
- list and switch mihomo policy groups from the CLI when mihomo is running;
- inspect imported mihomo proxy/rule provider status from the CLI;
- inspect current mihomo connections from the CLI;
- attribute current active-session traffic to OpenSurge DHCP devices in the
  Web GUI, including per-device connection counts, upload/download, and the
  dominant mihomo egress chain;
- inspect runtime log paths and recent dnsmasq/mihomo log lines from the CLI;
- collect a partial-failure JSON snapshot for UI and diagnostics surfaces;
- validate risky network behavior in an isolated virtual LAN before touching a
  normal LAN segment.

## Per-device policies

One mihomo process can apply independent policies to registered LAN devices.
OpenSurge gives each device a MAC-backed fixed IPv4 DHCP lease, then emits
per-device mihomo selector groups and `SRC-IP-CIDR` rules. The optional JSON
policy file supports a device default egress, direct device-specific actions
such as `REJECT`, and later domain/IP/protocol/port/rule-provider overlays.

OpenSurge intentionally ships no household templates or third-party rule
lists. Operators supply their own policy content; the empty starter file is
valid. See [per-device policy overlays](docs/device-policy.md) for the JSON
model, precedence, CLI commands, and validation boundary.

## Transparent proxying

TUN is the supported transparent proxy path on macOS. Mihomo `redir-port` and
PF TCP redirection are intentionally unsupported because the current Darwin
build reports redir as unsupported at runtime. Keep `mihomo.redir_port` and
`pf.redirect_tcp_to` at `0`; enable transparent proxying with
`transparent.mode: "tun"`.

## Mihomo profiles

OpenSurge for Mac can render a managed mihomo config or import the proxy and
rule sections from an existing mihomo profile. In imported mode, OpenSurge keeps
owning the gateway-critical fields: LAN binding, `allow-lan`, DNS, TUN,
`external-controller`, and runtime paths. The imported profile contributes only
engine-level sections such as `proxies`, `proxy-providers`, `proxy-groups`,
`rule-providers`, and `rules`.

```yaml
mihomo:
  profile_mode: "imported"
  profile: "./profiles/home.yaml"
```

Relative `mihomo.profile` paths are resolved from the OpenSurge config file's
directory. Relative `path:` entries inside imported `proxy-providers` and
`rule-providers` are resolved from the imported mihomo profile's directory.
OpenSurge renders `profile.store-selected: true` so mihomo can persist policy
group choices across restarts.

Preview the final generated mihomo config before starting gateway services:

```sh
go run ./cmd/omg doctor --config examples/config.imported-profile.example.yaml
go run ./cmd/omg render-mihomo --config examples/config.example.yaml
go run ./cmd/omg render-mihomo --config examples/config.imported-profile.example.yaml
```

Use `validate-mihomo` when `mihomo.binary` points to an installed mihomo binary.
It renders the final config and runs mihomo's own `-t` validation without
starting gateway services.

```sh
go run ./cmd/omg validate-mihomo --config examples/config.imported-profile.example.yaml
```

## Usage

```sh
go run ./cmd/omg doctor --config examples/config.example.yaml
go run ./cmd/omg status --config examples/config.example.yaml
go run ./cmd/omg status --config examples/config.example.yaml --format json
go run ./cmd/omg logs --config examples/config.example.yaml --tail 50 --format json
go run ./cmd/omg snapshot --config examples/config.example.yaml --tail 50 --format json
go run ./cmd/omg policies --config examples/config.imported-profile.example.yaml
go run ./cmd/omg policy-select --config examples/config.imported-profile.example.yaml --group Proxy --policy DIRECT
# after configuring device_policy.file:
go run ./cmd/omg devices --config ./config.yaml --format json
go run ./cmd/omg device-policy-select --config ./config.yaml --device alice-phone --slot default --policy DIRECT
go run ./cmd/omg connections --config examples/config.imported-profile.example.yaml --format json
go run ./cmd/omg providers --config examples/config.imported-profile.example.yaml --format json
go run ./cmd/omg provider-update --config examples/config.imported-profile.example.yaml --provider demo-provider --format json
go run ./cmd/omg render-mihomo --config examples/config.example.yaml
sudo go run ./cmd/omg start --config examples/config.example.yaml --format json
sudo go run ./cmd/omg stop --config examples/config.example.yaml --format json
```

`policy-select` first reads the live mihomo policy groups and rejects unknown
groups or policies before sending the selection change.
`provider-update --provider <name>` asks mihomo to refresh one proxy provider
and returns the refreshed provider state.
`logs --tail N --format json` includes recent `dnsmasq` and `mihomo` log lines
with per-file existence and read-error fields for control surfaces.
`snapshot --format json` aggregates status, doctor checks, leases, log tails,
policy groups, connections, and provider status; mihomo API failures are
reported inside the `mihomo` fields so the rest of the snapshot remains usable.
`start --format json` and `stop --format json` return a success payload with
`command`, `ok`, and `config_path` after the gateway action succeeds.
When `--format json` is used, command failures are emitted to stderr as
`{"command":"...","ok":false,"error":"..."}` while preserving the non-zero exit
code.

## Web GUI and menu bar app

The repository now includes the loopback Go Control API, an embedded React Web
GUI, and a read-only native SwiftUI menu bar launcher. For a development build:

```sh
make web-install
make control-build
./bin/opensurge-control --config examples/config.example.yaml
make menubar-build
```

The control service listens only on `127.0.0.1` and prints a one-time Web GUI
bootstrap link. The menu bar app shows status and recovery warnings and opens
the Web GUI; it deliberately has no start/stop or policy-selection actions.
See the [GUI architecture notes](docs/gui-architecture.zh-CN.md) for the current
security and packaging boundary.

`make gui-installer` builds a macOS package after requiring real mihomo and
dnsmasq binaries. Developer ID signing and notarization are opt-in through the
environment variables documented in the architecture notes; an unsigned local
package is never described as a release artifact.

Package upgrades refuse to run while same-LAN DHCP recovery is incomplete.
Before replacing payload files, preinstall stops the user control service and
menu bar app, runs the installed `omg stop`, and unloads the root helper. The
existing config, imported sources, policy data, and runtime history are kept;
only a first installation seeds `config.yaml` from the packaged example.

## Safety

`start` and `stop` are intended to run with `sudo` because they manage DHCP,
pf, and IPv4 forwarding. Runtime files are written under `runtime.dir` from the
config file.

## Development workflow

Use `make test` as the fast default gate. CI currently runs this unit-test gate
only, so ordinary pushes and pull requests do not need host networking,
passwordless sudo, Lima, or socket_vmnet.

Run `make lab-test` locally before committing or reviewing high-risk network
changes. This includes changes to DHCP, DNS, mihomo startup/config rendering,
pf rules, forwarding/rollback behavior, gateway lifecycle, lab scripts, and
example configs that affect runtime traffic. Keep the virtual LAN lab as a
local, nightly, or manual gate until a dedicated macOS runner can provide the
same controlled host privileges and network isolation.

Use `make lab-test-tun` for the supported transparent proxy path. That test
keeps clients proxy-free and requires mihomo to log the direct HTTPS connection
through its TUN inbound. Use `make lab-test-tun-imported-profile` when changing
mihomo profile import or overlay behavior; it runs the same TUN gate with an
imported profile fixture. Use `make lab-test-tun-imported-egress` when changing
imported provider or policy-selection behavior that should affect transparent
TUN traffic; it uses a local HTTP provider and controlled HTTP CONNECT proxy to
prove `policy-select` changes the TUN egress path between `DIRECT` and the
controlled proxy.

Use `make lab-test-tun-device-policy` when changing the MAC reservation,
per-device selector, or device override data path. It proves two clients get
their distinct fixed leases, independently select different TUN egress paths,
and that a device-specific domain `REJECT` takes effect. Domain/protocol rule
compilation, templates, and HTTP/MRS rule-provider configuration are covered by
unit tests; they do not require one Lab run per operator-defined rule.

Use `make policy-control-test` for policy-control and machine-readable CLI
changes. It starts the real mihomo binary without sudo, dnsmasq, pf, or TUN and
checks `policies`, invalid and valid `policy-select`, persisted selection
restore after mihomo restart, local DIRECT-vs-proxy egress switching through
mihomo's mixed-port, `connections`, `providers`, `provider-update` for file and
HTTP proxy providers, and `snapshot` against the live external-controller API.

Use `make same-lan-start-tun` and `make same-lan-adb-check` for the narrow
same-LAN default-gateway smoke. This gate keeps DHCP disabled, requires TUN, and
uses ADB to inspect one Android test device whose gateway and DNS point at the
Mac's LAN IP. Use `make same-lan-start-tun-proxy` with `OMG_SAME_LAN_*`
upstream-proxy environment overrides to prove one-domain real proxy egress, such
as `api.ipify.org`, before importing a full subscription. Use
`make same-lan-start-tun-imported-egress` plus
`make same-lan-adb-check-imported-egress` for the closer-to-real-device smoke
that imports a provider-backed `TunEgress` group, then switches same-LAN TUN
traffic from `DIRECT` to a controlled local HTTP CONNECT proxy. These gates do
not claim whole-LAN rollout readiness or real remote subscription exits.
When ADB is intentionally unavailable, the same imported egress evidence can
be collected with manual Android browser probes; see
[`tests/same-lan/README.md`](tests/same-lan/README.md#manual-phone-check-without-adb).

For a dedicated test Wi-Fi where router DHCP is manually disabled, use
`make same-wifi-dhcp-start-imported-egress`, then reconnect the Android client
in DHCP mode and run `make same-wifi-dhcp-adb-check-imported-egress`. This
separate high-risk runner uses `gateway.mode: "same_wifi_dhcp"` and requires an
explicit protected-static-address list plus an operator confirmation that router
DHCP is disabled. Its stop gate verifies OpenSurge cleanup, but router DHCP and
client automatic addressing must still be restored manually; see
[`tests/same-lan/WIFI-DHCP-RUNNER.md`](tests/same-lan/WIFI-DHCP-RUNNER.md).

## Virtual LAN lab

The integration lab runs the real macOS gateway against two lightweight Linux
clients. Lima provides the clients, while socket_vmnet creates an isolated
Layer 2 host network without a competing DHCP server. The test covers DHCP,
DNS, ICMP/NAT, direct HTTPS, and explicit HTTPS through mihomo `mixed-port`.

```sh
make lab-install
make lab-up
sudo -v
make lab-test
make lab-test-tun
make lab-test-tun-imported-profile
make lab-test-tun-imported-egress
make lab-test-tun-device-policy
make lab-down
```

The one-time installer adds a root-owned, fixed-function network helper and a
narrow sudoers rule for starting, stopping, and inspecting the lab network. The
gateway binary itself is not granted passwordless root access; refresh the sudo
ticket with `sudo -v` before an end-to-end test. See `tests/lab/README.md` for
the topology, safety checks, and troubleshooting steps.
