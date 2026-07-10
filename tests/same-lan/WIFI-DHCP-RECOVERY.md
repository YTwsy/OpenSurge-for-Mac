# same-WiFi DHCP Recovery Reference

[简体中文](WIFI-DHCP-RECOVERY.zh-CN.md) | English

This runbook is for a higher-risk test close to the Surge-style same-WiFi
gateway mode: the Mac and downstream devices stay on the main Wi-Fi, router DHCP
is disabled, and OpenSurge later provides gateway/DNS service on that same LAN.

The current automated same-LAN smoke still defaults to `dhcp.enabled: false` and
does not take over main-LAN DHCP. Only disable router DHCP on a dedicated test
Wi-Fi when you know how to reach the router admin page and have a recovery path.

## Record Before Testing

Save these details, preferably in screenshots or offline notes:

- Wi-Fi SSID and password;
- router admin address, for example `192.168.1.1`;
- router admin username and password;
- current router LAN IP, subnet mask, DHCP toggle, and DHCP pool;
- Mac Wi-Fi interface, usually `en0`;
- Mac current IPv4 address, router, and DNS;
- static IPv4 settings for at least one backup client.

Inspect Mac Wi-Fi state:

```sh
networksetup -listallhardwareports
ipconfig getifaddr en0
route -n get default
scutil --dns | sed -n '1,80p'
```

## Recommended Recovery Topology

Keep two ways to reach the router admin page:

- Mac with a static IPv4 address, such as `192.168.1.20/24`, router
  `192.168.1.1`;
- another phone or laptop prepared with static IPv4, such as `192.168.1.21/24`,
  router `192.168.1.1`.

This lets you open `http://192.168.1.1/` and turn router DHCP back on even if
router DHCP is off and OpenSurge DHCP did not start correctly.

## Set Static IPv4 On The Mac

If the main Wi-Fi is `192.168.1.0/24` and the router is `192.168.1.1`:

```sh
sudo networksetup -setmanual "Wi-Fi" 192.168.1.20 255.255.255.0 192.168.1.1
sudo networksetup -setdnsservers "Wi-Fi" 192.168.1.1 1.1.1.1
```

Confirm router access:

```sh
ping -c 3 192.168.1.1
open http://192.168.1.1/
```

If your Wi-Fi service is not named `Wi-Fi`, use
`networksetup -listallnetworkservices` to find the actual service name.

## Immediately After Disabling Router DHCP

After disabling router DHCP, do not change more settings yet. First confirm:

```sh
ping -c 3 192.168.1.1
curl --max-time 3 http://192.168.1.1/ >/dev/null || true
```

If the Mac can no longer reach the router, restore router DHCP before starting
OpenSurge.

## If The Test Loses Connectivity

Recover in this order and avoid factory reset unless necessary:

1. Keep the Mac on the original Wi-Fi.
2. Put the Mac back on a static address in the router subnet:

   ```sh
   sudo networksetup -setmanual "Wi-Fi" 192.168.1.20 255.255.255.0 192.168.1.1
   sudo networksetup -setdnsservers "Wi-Fi" 192.168.1.1 1.1.1.1
   ```

3. Open `http://192.168.1.1/` and re-enable router DHCP.
4. If the Mac cannot reach the admin page, use the backup phone or laptop with a
   static IP.
5. If that still fails, connect to a router LAN port over Ethernet and use a
   static IP.
6. Use hardware reset only if the admin address, password, or router state
   cannot be recovered.

## Restore After Testing

Stop OpenSurge first, then restore router DHCP:

```sh
make same-wifi-dhcp-stop
```

In the router admin page, re-enable DHCP and confirm the pool matches the values
recorded before the test.

Then put the Mac Wi-Fi service back on DHCP:

```sh
sudo networksetup -setdhcp "Wi-Fi"
sudo networksetup -setdnsservers "Wi-Fi" Empty
sudo ipconfig set en0 DHCP
```

Reconnect Wi-Fi:

```sh
networksetup -setairportpower en0 off
sleep 2
networksetup -setairportpower en0 on
```

Confirm recovery:

```sh
ipconfig getifaddr en0
route -n get default
scutil --dns | sed -n '1,80p'
ping -c 3 192.168.1.1
curl --fail --silent --show-error --max-time 5 https://example.com/ >/dev/null
```

## Client Recovery

If a phone or test client was manually set to static IP, gateway, or DNS, switch
it back to automatic addressing:

- iOS/iPadOS: Wi-Fi details -> Configure IP -> Automatic; Configure DNS ->
  Automatic.
- Android: Wi-Fi details -> IP settings -> DHCP; Proxy -> None.
- macOS client: `sudo networksetup -setdhcp "Wi-Fi"`.

If the client still cannot get an address, forget the Wi-Fi network and join it
again.

## Safety Boundary

- Do not first test this mode on your daily main Wi-Fi; use a dedicated test
  SSID when possible.
- Do not let both router DHCP and OpenSurge DHCP serve the same LAN.
- Do not expose the OpenSurge control surface bare on the LAN; remote control
  needs token/auth.
- Any "whole-home works" claim must include recovery evidence: OpenSurge stopped,
  router DHCP restored, Mac back on DHCP, and at least one client automatically
  getting an address and reaching the internet.
