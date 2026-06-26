# Open Mihomo Gateway for macOS

Open Mihomo Gateway is a macOS CLI gateway MVP. It prepares a Mac to act as an
IPv4 LAN gateway, runs dnsmasq for DHCP/DNS, runs mihomo for proxying, and uses
macOS pf for NAT.

The current implementation is milestone-driven:

1. CLI, config, runtime state, and basic status/doctor commands.
2. dnsmasq config, process management, and lease parsing.
3. mihomo config, process management, and version API checks.
4. pf anchor management and IPv4 forwarding restore.

Mihomo `redir-port` is disabled by default on macOS because the current Darwin
build reports redir as unsupported at runtime. The virtual LAN lab validates
NAT, DHCP/DNS, and mihomo `mixed-port`; transparent TCP capture should be
designed as a separate TUN or engine-specific milestone. An experimental TUN
POC can be enabled with `transparent.mode: "tun"`.

## Usage

```sh
go run ./cmd/omg doctor --config examples/config.example.yaml
go run ./cmd/omg status --config examples/config.example.yaml
sudo go run ./cmd/omg start --config examples/config.example.yaml
sudo go run ./cmd/omg stop --config examples/config.example.yaml
```

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

Use `make lab-test-tun` for the experimental transparent proxy path. That test
keeps clients proxy-free and requires mihomo to log the direct HTTPS connection
through its TUN inbound.

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
make lab-down
```

The one-time installer adds a root-owned, fixed-function network helper and a
narrow sudoers rule for starting, stopping, and inspecting the lab network. The
gateway binary itself is not granted passwordless root access; refresh the sudo
ticket with `sudo -v` before an end-to-end test. See `tests/lab/README.md` for
the topology, safety checks, and troubleshooting steps.
