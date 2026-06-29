# Real-device isolated LAN smoke test

[简体中文](README.zh-CN.md) | English

This guide is for the first real-device milestone after the virtual LAN lab has
passed. The goal is to validate the same macOS gateway implementation with
physical clients while keeping the test network isolated from the home or office
LAN.

## Core topology

Yes: the core idea is to give the Mac a dedicated downstream LAN interface. Test
devices join only that downstream LAN, while another Mac interface remains the
upstream Internet path.

```text
Home router / main Wi-Fi
        ^
        |
   Mac Wi-Fi en0
        |
   omg + mihomo + dnsmasq + pf
        |
   Mac USB Ethernet en7: 192.168.50.1
        v
   Test switch / spare router in AP or bridge mode
        v
   iPhone / PS5 / Switch / test laptop
```

In this topology the home router's DHCP server can remain enabled because it is
on a different broadcast domain. The project's dnsmasq instance is configured to
bind to the downstream interface only, so its DHCP broadcasts should stay on
`en7` and should not appear on the main Wi-Fi.

Never run this project's DHCP server on the main home or office LAN.

## Hardware requirements

- A Mac with one upstream interface, normally Wi-Fi such as `en0`.
- A separate downstream interface, normally a USB Ethernet adapter such as `en7`.
- A test switch, or a spare router configured as AP/bridge mode.
- One test laptop, plus one or more devices such as iPhone, PS5, or Switch.

The spare router must not run DHCP, NAT, firewall, or router mode for this test.
It should only bridge Wi-Fi/Ethernet clients onto the Mac's downstream LAN.

## Preflight

Run the virtual LAN gates first:

```sh
make lab-up
sudo -v
make lab-test
make lab-test-tun
make lab-down
```

Identify the Mac interfaces:

```sh
networksetup -listallhardwareports
route -n get default
ifconfig en7
```

Expected:

- `upstream_interface` is the interface that reaches the Internet, for example
  `en0`.
- `gateway.interface` is the downstream test LAN, for example `en7`.
- The two interfaces must be different.
- The upstream network must not already use `192.168.50.0/24`.

Bring up the downstream LAN address:

```sh
sudo ifconfig en7 inet 192.168.50.1 netmask 255.255.255.0 up
```

## Configs

Create local configs under `runtime/real-device/`. Keep these files out of
commits if they contain machine-specific interface names or proxy settings.
The example below assumes commands are run from the repository root.

Use explicit proxy mode first:

```yaml
gateway:
  interface: "en7"
  lan_ip: "192.168.50.1"
  upstream_interface: "en0"

dhcp:
  binary: "./runtime/tools/bin/dnsmasq"
  enabled: true
  range_start: "192.168.50.100"
  range_end: "192.168.50.200"
  lease_time: "30m"
  domain: "realtest"

dns:
  listen: "192.168.50.1"
  port: 53
  upstream: ""

mihomo:
  binary: "./runtime/tools/bin/mihomo"
  config: "./runtime/real-device/mihomo.yaml"
  mixed_port: 17890
  redir_port: 0
  api_addr: "127.0.0.1:19090"
  secret: ""

pf:
  anchor_name: "com.apple/open_mihomo_gateway_real_device"
  redirect_tcp_to: 0

transparent:
  mode: "off"
  tun_device: "utun123"
  tun_stack: "mixed"
  tun_auto_route: true
  tun_auto_detect_interface: false
  tun_strict_route: false

runtime:
  dir: "./runtime/real-device"
```

For transparent TUN mode, copy that config and change only these fields:

```yaml
dns:
  listen: "192.168.50.1"
  port: 53
  upstream: "127.0.0.1#1053"

transparent:
  mode: "tun"
```

The current generated mihomo config uses `MATCH,DIRECT`. This milestone proves
that real-device traffic is captured and forwarded by the Mac gateway. It does
not yet prove subscription routing or remote proxy egress behavior.

## Explicit proxy smoke

Build and start:

```sh
GOCACHE=/private/tmp/omg-go-cache go build -o bin/omg ./cmd/omg
sudo ./bin/omg doctor --config runtime/real-device/config-off.yaml
sudo ./bin/omg start --config runtime/real-device/config-off.yaml
./bin/omg status --config runtime/real-device/config-off.yaml
./bin/omg leases --config runtime/real-device/config-off.yaml
```

Connect the test laptop and devices to the downstream AP. The laptop should get
an address such as `192.168.50.100`, with router and DNS set to `192.168.50.1`.

On the test laptop:

```sh
route -n get default
dig @192.168.50.1 example.com A
curl --noproxy '*' --fail --show-error https://example.com/
curl --proxy http://192.168.50.1:17890 --fail --show-error https://example.com/
```

On iPhone, PS5, Switch, or another device:

- Confirm the device address is `192.168.50.x`.
- Confirm router/DNS is `192.168.50.1` when the device exposes those details.
- For explicit proxy testing, set HTTP proxy to `192.168.50.1:17890`.
- Open a simple HTTPS page or perform a normal connectivity check.

Stop and verify cleanup:

```sh
sudo ./bin/omg stop --config runtime/real-device/config-off.yaml
./bin/omg status --config runtime/real-device/config-off.yaml
sysctl -n net.inet.ip.forwarding
sudo pfctl -s Anchors
```

## Transparent TUN smoke

Start the TUN config:

```sh
sudo ./bin/omg start --config runtime/real-device/config-tun.yaml
./bin/omg status --config runtime/real-device/config-tun.yaml
./bin/omg leases --config runtime/real-device/config-tun.yaml
```

Leave clients without explicit proxy settings. From the test laptop:

```sh
dig @192.168.50.1 example.com A
curl --noproxy '*' --fail --show-error https://example.com/
```

On the Mac, confirm mihomo saw client traffic:

```sh
tail -n 120 runtime/real-device/logs/mihomo.log
```

Expected: the log contains a TCP connection from a real client address such as
`192.168.50.100` to `example.com:443`, or to the target used during the smoke
test.

Stop and verify cleanup:

```sh
sudo ./bin/omg stop --config runtime/real-device/config-tun.yaml
./bin/omg status --config runtime/real-device/config-tun.yaml
sysctl -n net.inet.ip.forwarding
sudo pfctl -s Anchors
```

## Acceptance checklist

- The main home or office network remains unaffected.
- DHCP leases are issued only to devices on the downstream test LAN.
- Test laptop receives `192.168.50.x`, router `192.168.50.1`, and DNS
  `192.168.50.1`.
- Direct HTTPS works through NAT in explicit proxy mode.
- Explicit HTTPS through `192.168.50.1:17890` works.
- TUN mode works without client proxy settings.
- `mihomo.log` shows real client traffic in TUN mode.
- `stop` removes runtime state and unloads the pf anchor.
- IP forwarding is restored to its previous value after `stop`.

## Artifact checklist

Create one artifact directory per run:

```sh
mkdir -p artifacts/real-device/$(date +%Y%m%d-%H%M%S)
```

Save:

- `config-off.yaml` and `config-tun.yaml`.
- `host-before.txt`: `route -n get default`, downstream `ifconfig`, pf anchors,
  and `sysctl -n net.inet.ip.forwarding`.
- `doctor-off.txt` and `doctor-tun.txt`.
- `start-off.log` and `start-tun.log`.
- `status-during.txt` and `leases.txt`.
- `mihomo.log`.
- `client-laptop.txt`: route, DNS, curl results.
- `client-devices.md`: device model, IP, router/DNS, explicit proxy result, TUN
  result.
- `host-after.txt`: status, pf anchors, forwarding, and any leftover processes.

## Abort conditions

Stop immediately and collect diagnostics if any of these happen:

- A non-test device on the main LAN receives `192.168.50.x`.
- The Mac upstream interface and downstream interface are the same.
- The spare router is still running DHCP or NAT.
- The upstream network already uses `192.168.50.0/24`.
- `stop` fails to unload the pf anchor or restore IP forwarding.
- Client traffic works only through the router's own NAT instead of through the
  Mac gateway.
