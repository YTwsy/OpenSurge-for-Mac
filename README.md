# Open Mihomo Gateway for macOS

Open Mihomo Gateway is a macOS CLI gateway MVP. It prepares a Mac to act as an
IPv4 LAN gateway, runs dnsmasq for DHCP/DNS, runs mihomo for proxying, and uses
macOS pf for NAT and TCP transparent redirect.

The current implementation is milestone-driven:

1. CLI, config, runtime state, and basic status/doctor commands.
2. dnsmasq config, process management, and lease parsing.
3. mihomo config, process management, and version API checks.
4. pf anchor management and IPv4 forwarding restore.

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
