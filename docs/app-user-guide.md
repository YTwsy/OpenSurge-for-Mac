# OpenSurge for Mac App User Guide

[简体中文](app-user-guide.zh-CN.md) · **English**

This short guide is for people using the packaged OpenSurge for Mac app. It
covers installation, source import, gateway startup, and safe network recovery.
CLI and development workflows are intentionally left out.

![OpenSurge for Mac dashboard](images/opensurge-dashboard.png)

## Install and open the app

1. Download the package for your Mac from
   [GitHub Releases](https://github.com/YTwsy/OpenSurge-for-Mac/releases): use
   `arm64-unsigned.pkg` on Apple Silicon or `x86_64-unsigned.pkg` on Intel.
2. Double-click the package. If macOS blocks it, open **System Settings →
   Privacy & Security** and choose **Open Anyway** for this package. You do not
   need to disable Gatekeeper globally.
3. Open **OpenSurge** from `/Applications`; the app opens its menu bar status
   panel directly.
4. Later, either open **OpenSurge** again or click its menu bar icon to show the
   same panel, then choose **打开 OpenSurge 面板** (Open the OpenSurge panel)
   to launch the Web GUI.

The menu bar app shows status and recovery warnings and opens the Web GUI.
Sources, network settings, devices, and policies are managed in the Web GUI.

## First-time setup

### 1. Import a source

Open **来源** (Sources) and import either an HTTPS subscription or a local
mihomo YAML file:

1. Select **导入为草稿** (Import as draft).
2. Confirm that structural validation succeeds.
3. If the gateway is stopped, select **设为下次启动版本** (Use on next start).
   If it is running, you can apply the source and reload the gateway.

Importing a draft does not change the current network. Applying a source while
the gateway is running validates the full configuration before briefly
restarting gateway services.

### 2. Choose a network mode

Open **网络设置** (Network Settings) and choose the topology that matches your
deployment:

| Mode | Best for | Main requirement |
| --- | --- | --- |
| Same-LAN DHCP takeover | Automatically routing a home LAN through OpenSurge | Follow the guided router-DHCP shutdown and recovery flow |
| Same-LAN manual gateway | Trying OpenSurge with a few devices | Keep router DHCP enabled and point device gateway/DNS to the Mac |
| Isolated downstream LAN | A separate AP, SSID, or VLAN | Let the Mac serve the dedicated downstream network |

Set the downstream and upstream interfaces, Mac gateway IPv4, DHCP pool, and
upstream DNS. Keep **mihomo TUN** enabled for transparent proxying. Enable
**每设备策略** (Per-device policies) if devices need independent egress
choices, then select **保存网络配置** (Save network configuration).

### 3. Start OpenSurge

For **Same-LAN DHCP takeover**, gateway start and stop are part of the recovery
state machine. Follow the steps shown in Network Settings:

1. Select **保存网络快照与离线恢复卡** to save the network snapshot and offline
   recovery card.
2. Switch the Mac to a fixed IPv4 address.
3. Disable router DHCP when prompted.
4. Return to OpenSurge and run the DHCP OFFER probe.
5. After the probe succeeds, select **启动 OpenSurge** (Start OpenSurge).
6. Reconnect a client and complete the DHCP, DNS, and TUN validation step.

Do not quit immediately after disabling router DHCP. OpenSurge keeps the
recovery state active until the network has actually been restored.

## Everyday use

- **总览** (Dashboard) shows gateway state, active devices, and the latest 60
  seconds of traffic trends.
- **来源** (Sources) refreshes subscriptions and shows version differences. A
  refresh creates a draft that still needs to be applied.
- **设备** (Devices) registers devices and lets each one follow global rules or
  use an independent egress.
- **策略** (Policies) tests proxy health and switches applied Selectors
  immediately.
- **连通性** (Connectivity) shows latency, matched rules, and egress chains
  through the currently applied mihomo path.
- **诊断** (Diagnostics) shows recent operations, connections, providers, and
  redacted logs.

Green **即时生效** (Applies immediately) controls switch an already-applied
egress. Changes to device identity, candidates, or rules must be saved and then
applied through a gateway reload.

Proxy health and connectivity tests originate from the gateway Mac. They help
confirm that the proxy configuration works, but they do not replace DHCP, DNS,
and TUN validation from a downstream device.

## Stop and restore the network

For **Same-LAN DHCP takeover**, stopping the gateway is also part of the
recovery state machine:

1. Complete client validation or explicitly record that it was skipped.
2. Select **停止 OpenSurge** (Stop OpenSurge).
3. Re-enable router DHCP when prompted.
4. Return to OpenSurge and run the DHCP OFFER probe.
5. Restore automatic DHCP on the Mac, or explicitly keep the static IPv4.
6. Confirm that recovery is complete before quitting OpenSurge.

The menu bar provides two different quit actions:

- **只退出菜单栏 App** (Quit Menu Bar App Only) closes only the menu bar icon.
  Gateway and background services keep running.
- **退出 OpenSurge** (Quit OpenSurge) is available only after the gateway is
  stopped and no recovery action remains. It quits the menu bar app and user
  Control Service.

## Common issues

**The Web GUI does not open**

Select **重新连接** (Reconnect) from the menu bar. This restarts only the user
Control Service and does not stop a running gateway data plane.

**The start flow cannot continue**

Read the blockers in **网络设置** (Network Settings). Check the interfaces, Mac
gateway IPv4, protected addresses, and DHCP pool, and make sure all changes are
saved.

**A device shows no traffic**

Generate new traffic from the device and refresh the Dashboard. The UI shows
active sessions and the latest 60-second trend, not long-term traffic history.

**Network recovery remains incomplete**

Open **网络设置** (Network Settings) and continue the recovery flow. A stopped
gateway does not by itself mean that router DHCP and the Mac network settings
are restored.
