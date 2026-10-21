# OpenSurge for Mac App User Guide

[简体中文](app-user-guide.zh-CN.md) · **English**

This short guide is for people using the packaged OpenSurge for Mac app. It
covers installation, proxy configuration, gateway startup, and safe network
recovery. CLI and development workflows are intentionally left out.

Desktop instructions here describe v0.3.0 and later. The v0.2.4 release uses the
legacy Swift menu bar host; see [installer acceptance](../agent-wiki/sources/validation/evidence-map.md#桌面与安装) for the
verified scope and outstanding platform checks.

![OpenSurge for Mac dashboard](../images/opensurge-dashboard.png)

## Install and open the app

1. Open the latest stable release on
   [GitHub Releases](https://github.com/YTwsy/OpenSurge-for-Mac/releases), expand **Assets**
   and download the `.pkg` for your Mac: `arm64` for Apple Silicon or `x86_64` for Intel.
   The v0.2.4 menu bar workflow is described in its
   [versioned guide](https://github.com/YTwsy/OpenSurge-for-Mac/blob/v0.2.4/docs/app-user-guide.md).
2. Double-click the package. If macOS blocks it, open **System Settings →
   Privacy & Security** and choose **Open Anyway** for this package. You do not
   need to disable Gatekeeper globally.
3. Open **OpenSurge** from `/Applications` to show the independent main window.
4. The Dock, another launch, or **Open OpenSurge Dashboard** in the tray returns
   to the same window. Clicking the menu bar icon opens a separate status panel;
   sources, networking, devices and policies are managed in the main window.

First launch requests 1440 × 900 logical points, fitted to the available screen.
You can resize the window and the app remembers your choice. Drag the empty top
area; native traffic lights blend into the sidebar. Closing hides the window.
`⌘Q` confirms quitting only the desktop app while background services and the
running gateway continue.

**App Settings & Updates** in the tray provides explicit login-item management.
If macOS requires approval, open Login Items through the provided action. Upgrades
do not automatically toggle registration; the actual OS status is authoritative.

The menu bar panel automatically checks once for the latest stable release and
also provides a manual **检查更新** (Check for Updates) action. When an update is
available, **打开下载页** opens that version's GitHub Release page. The app does
not download or install the PKG itself; choose the package for the Mac's
architecture and follow the installation steps above. Existing configuration
and data are preserved without uninstalling first.
Prerelease builds display their complete `rc.N` version. They are not offered an
older stable release, while the stable release with the same base version is
still detected when it becomes available.

## Manage from another LAN device

LAN management is off by default. In a build that includes this feature, open
**App Settings & Updates → LAN management** on the Mac:

1. Turn on **Allow LAN management** and select the Mac IPv4 address on the LAN
   shared with your phone or other device.
2. Set and confirm a management password of 12–128 characters; spaces are allowed.
   The default port is `61768`. Change it under **Advanced settings** if occupied.
3. Select **Save and enable**. Once the status says **Listening**, copy the URL or
   scan its QR code from a device on the same subnet.
4. Enter the management password in that device's browser to open the shared
   control panel. The QR contains only the URL; it never carries a password or logs you in.

The endpoint uses HTTP, without transport encryption for passwords or sessions.
Enable it only on a trusted LAN. This version supports private IPv4 addresses;
IPv6 management URLs, HTTPS and public access are not provided. The browser can
manage status, sources, devices, policies and networking. Finder actions, copying
Mac file paths and exporting local copies remain available only on the Mac.

In a remote browser, expand quick settings at the bottom of the sidebar for
**Change management password** and **Sign out**. These actions are hidden on the
gateway Mac. Changing the password requires the current password and signs out every LAN browser. If you
forget it, use **Reset management password** in the Mac settings. Sessions require
a new login after 12 idle hours or a Control Service restart. Authentication loss
never automatically replays an earlier network operation.

An interface, IPv4 or prefix change pauses access. Select and save the address
again on the Mac; an old QR may no longer work. Remote operations such as restoring
DHCP can disconnect your browser, so check the actual result on the Mac. Turning
off and saving **Allow LAN management** releases the port and signs out all LAN
sessions. Quitting only the desktop app leaves the Control Service and enabled LAN
entry running. Stopping the gateway does not close this management entry.

## First-time setup

### 1. Prepare the proxy configuration

Importing mihomo YAML is an optional data source, not a prerequisite for using
OpenSurge. For the first setup, use either of these paths or combine them:

- Open **来源** (Sources) and import either an HTTPS subscription or a local
  mihomo YAML file. Select **选择或拖入 YAML** (Choose or drop YAML), or drop a
  file directly onto the same area. This is the page's entry point for importing
  a complete YAML source. Select **导入为草稿** (Import as draft) and confirm
  that structural validation succeeds. If the gateway is stopped, you can
  select **设为下次启动版本** (Use on next start); if it is running, you can
  apply the source and reload the gateway.
- Expand **Advanced: Global Profile Overlay**, collapsed by default after the
  source list. Even with no imported source, a valid global overlay can define
  personal proxies, providers, proxy groups, rules, and DNS operations.
  OpenSurge supplies the managed base needed for this standalone configuration,
  so it follows the same preview and startup flow as an imported source.

The ordinary overlay editor provides two common operations: high-priority
custom rules fixed before other rules, and custom proxies added from `ss://`,
`vmess://`, `vless://`, `trojan://`, `hysteria2://`, HTTP, or SOCKS5 share
links. A small manual form remains for HTTP/SOCKS5. Share links here add proxies
only; they do not create a subscription or another complete YAML source.
Low-priority tail rules, providers, proxy groups, and DNS operations are
maintained in **Expert Overlay YAML**.

Saving a source or global-overlay draft does not immediately change the current
network. OpenSurge composes the final configuration from whichever inputs are
actually present: the selected source, the global overlay, and the managed
Tailscale Exit Node when enabled. Missing any of these inputs is normal. For
example, a valid global overlay remains fully usable when there is no source and
no Tailscale configuration.

While the gateway is stopped, open **策略** (Policies) to inspect the groups and
proxies in that final composition, choose members of manual Selectors, and test
proxy latency. A non-takeover prepared mihomo instance provides real Provider
expansion, selection, and health testing. Visiting Policies is not a startup
prerequisite: even if it has never been opened, selecting **启动 OpenSurge**
(Start OpenSurge) in the Web GUI recomposes the same configuration, validates
it once in the final runtime context, then saves and starts it. Previewing,
selecting, and testing do not replace the saved startup configuration. Saving
an overlay while running leaves actual routing unchanged; an overlay-only draft
is adopted on the next App start. The maintenance command `sudo omg start` uses
only persisted configuration and does not read Web drafts. To inspect exact
changes, use the final-config
preview on Sources to compare the raw source, overlay changes, effective source,
and final mihomo config. Refreshing a subscription only creates a new draft and
never changes the running gateway automatically.

**DNS references and sources** in the final-config preview lists resolver
targets, binding results and their provenance. Confirmed references from node
resolution to the subscription's own DNS are bound to the current core without
enabling the global overlay. While the gateway is stopped, the prepared core
opens a private loopback DNS listener only when that dependency exists, and
checks both TCP and UDP readiness. This does not start the gateway or change
system DNS. Independent local and external resolvers retain their targets.
Follow specific diagnostics for ambiguous references, cycles or missing original
listener metadata; another proxy client's running DNS is not compatibility
evidence. Preview is configuration inference; node tests and the relevant
network gates establish runtime behavior. Supported forms and explicit service
declarations are described in the
[DNS reference contract](../agent-wiki/sources/decisions/mihomo-profile-overlay.md#dns-reference-composition-and-binding).

Each imported-source card shows the local snapshot managed by OpenSurge. You
can copy its full path or select **Finder 中显示** (Show in Finder) to reveal
the file. Do not edit the managed snapshot because its digest, history, and
apply state belong to OpenSurge. Select **导出副本** (Export copy) instead:
OpenSurge writes a separate `0600` YAML file under
`~/Library/Application Support/OpenSurge/exports/`, then opens Finder with the
new file selected. Refreshing the source does not overwrite exported copies,
and an edited copy must be imported as a local file before it becomes a draft.

### 2. Choose a network mode

Open **网络设置** (Network Settings) and choose the topology that matches your
deployment:

| Mode | Best for | Main requirement |
| --- | --- | --- |
| Same-LAN DHCP takeover | Automatically routing a home LAN through OpenSurge | Follow the guided router-DHCP shutdown and recovery flow |
| Same-LAN manual gateway | Trying OpenSurge with a few devices | Keep router DHCP enabled and point device gateway/DNS to the Mac |
| Isolated downstream LAN | A separate AP, SSID, or VLAN | Let the Mac serve the dedicated downstream network |

Set the downstream and upstream interfaces, Mac gateway IPv4, DHCP pool, and
upstream DNS. Keep **mihomo TUN** enabled for transparent proxying. Per-device
policies are enabled by default; register devices and choose their egress on the
Devices page. Select **保存网络配置** (Save network configuration) to save network settings.

All three topologies expose the two experimental IPv6 settings. **IPv6 DNS
queries** controls upstream and Fake-IP AAAA answers; explicit IPv6 mappings in
enabled hosts can still answer AAAA. **Downstream IPv6 takeover** establishes
the userspace TCP/UDP path. **Auto** takes over only when the upstream has a
public IPv6 address (ULA does not count) and an IPv6 default route. **Always**
can establish the downstream path without it, but real public-IPv6 `DIRECT`
traffic still needs an upstream route.

An isolated downstream LAN publishes RA/SLAAC/RDNSS automatically. Same-LAN
DHCP takeover can publish them to the whole LAN after main-router IPv6
RA/DHCPv6 is disabled or RA Guard is in place; the page requires an explicit
readiness confirmation. OpenSurge uses the normal Medium router preference.
Bypass-router mode sends no RA. Its client setup card shows the stable IPv4,
IPv6 ULA, and the same Mac link-local address for the default gateway and DNS on selected
clients; remove the competing main-router IPv6 default route on those clients.

If SafeDNS, DNS Proxy, content filtering, or another Network Extension causes
local-Mac DNS or connectivity failures with TUN alone, enable **Mac local
system-proxy coordination** in the same form. After the gateway is ready,
OpenSurge points HTTP and HTTPS proxy settings for the current upstream network
service at the local mihomo mixed-port, then restores the pre-start state on
stop, startup rollback, or a failed mihomo restart. This option is off by
default, requires TUN, and affects only Mac applications that honor system
proxy settings; it does not replace TUN or change downstream devices. Startup
is rejected when HTTP/HTTPS proxying, PAC, or proxy auto-discovery is already
active, rather than overwriting those settings.

### Optional: Custom DNS

Expand **Sources → Custom DNS**, beside and independently enabled from the Global
Profile Overlay. Network Settings retains draft status, mapping/policy counts and
an application status; **Manage custom DNS** opens and focuses the Sources editor.
Import a hosts file or Mihomo YAML
containing only `hosts` / `dns`. This supports fixed service addresses, corporate
DNS routing and real-IP exceptions for Mac TUN and downstream devices using
OpenSurge DNS. Arbitrary dnsmasq/Surge configurations, remote subscriptions,
scripts, mDNS, bypass devices and dynamic DHCP name synchronization are outside
this feature.

Both formats use UTF-8 and have a 2 MiB limit. A hosts file contains a literal IP
followed by names, with optional `#` comments:

```text
192.168.50.10 nas.home.arpa storage.home.arpa
2001:db8::10 nas.home.arpa
```

Repeated names accumulate unique addresses. Replace the documentation IPv6
address with a reachable one. With hosts enabled, an explicit IPv6 mapping takes
precedence and can answer AAAA even when **IPv6 DNS queries** is off. That setting
controls upstream and Fake-IP AAAA answers; it does not remove manually specified
addresses. An answer does not establish IPv6 reachability: configure and verify
the Mac local IPv6 path and downstream IPv6 takeover separately. To make a fixed
name IPv4-only, retain only its IPv4 hosts mapping.
Hosts import replaces the draft's whole hosts section while retaining
DNS settings. YAML import replaces only the sections it includes. Review the
replacement, choose **Accept imported content**, then **Save draft**.

**Download example** provides a combined fixed-address/corporate DNS fragment:

```yaml
hosts:
  nas.home.arpa: 192.168.50.10
dns:
  nameserver-policy:
    '+.corp.example': [10.20.0.53]
  fake-ip-filter-mode: blacklist
  fake-ip-filter:
    - '+.corp.example'
```

The policy chooses the upstream; blacklist entries request real answers. This
creates no VPN, routes, NAS addresses or DHCP reservations: the resolver and
service must already be reachable. If the source uses whitelist/rule mode, this
example explicitly replaces its filter mode and list; inspect **Preview** first.
Within the same mode, blacklist/whitelist append unique entries; rule mode prepends
custom rules. Whitelist matches mean Fake-IP, not real-IP exceptions.

Custom values override matching subscription/global-overlay keys; removing an
override restores inheritance. IP and resolver lists replace the whole value for
that key. Hosts take precedence over upstream policy and can also affect node and
provider resolution; more specific names may win over wildcards.

Enabling requires the managed DNS upstream, TUN auto-route and Mac system DNS
coordination. While DHCP is enabled, single-label names and the DHCP suffix
(default `lan`) remain reserved for lease resolution. Custom hosts, upstream
policies and alias chains cannot cover that namespace. Preview lists application
blockers; a structurally valid draft can still be saved. While active, Mihomo's
`use-system-hosts` controls system-hosts participation; `/etc/hosts` is not modified.

**Saving a draft does not change the running gateway.** The current source shows
pending DNS changes. Apply from its card or the Custom DNS section: validation and
confirmation precede a running gateway reload, or saving for the next start when
stopped. If no exact current source row can be reused, follow the Network Settings
link and use the existing stop/start workflow. This never selects an unused source
or a refreshed subscription version automatically. Disabling also requires saving and
applying. Stopped policy preparation uses the saved draft; running policy views
continue to show applied configuration. **Export draft** contains only the custom
YAML: the desktop app opens a native save dialog and the browser downloads a file.
A successful preview does not establish live DNS/connection acceptance on either
entry path.

### Optional: External DNS filtering

With the gateway stopped, open **Network Settings → Advanced Mihomo / DNS settings
→ External DNS filtering**, enable the option and enter your own service endpoint,
for example `192.168.50.2:5353` or `[2001:db8::2]:5353`. Use a literal IP and port.
The service must support ordinary UDP DNS; truncated replies are retried over TCP.
Keep downstream DNS upstream at `127.0.0.1#1053`, save, then start the gateway.
This option is off by default and requires the OpenSurge core containing this
feature; older cores explicitly fail to start with filtering enabled.

Managed DNS queries are checked before the existing hosts, Fake-IP and resolver
logic. A normal external answer permits that logic; its real IP does not replace
Fake-IP. NXDOMAIN, REFUSED, SERVFAIL, empty answers and sink replies containing
`0.0.0.0` / `::` are returned directly. Configure any other sink IPs under
**Additional blocking IPs**. The default timeout is 2000 ms, adjustable from 100 to
10000 ms. An unavailable service fails the query instead of automatically allowing it.

The service needs its own upstreams and a direct route from the Mac. Never forward
it back to OpenSurge. Ordinary DNS has no universal advertising marker, so configure
sink IPs to match the service's rules. Current validation uses a controlled service
and isolated core; real Numa / SmartDNS versions and host-network paths remain unverified.

**Domain suffixes excluded from filtering** includes each domain and its subdomains, separated by
commas. Single-label names, `.local`, `.localhost`, `.lan`, `home.arpa`, reverse
queries and configured DHCP / system search / MagicDNS namespaces retain their
local path. Add suffixes for other private hosts names; public fixed hosts names
are still checked first. While enabled, dnsmasq caching and system-hosts shortcuts
are disabled, while DHCP lease names remain available.

Mac coverage depends on existing system DNS and TUN capture. Client caches,
application DoH, direct IP traffic, existing connections and proxy requests that
never use managed DNS are outside scope. To disable, stop the gateway, clear the
switch, save and start again. Client-side cached answers still need to expire or
be refreshed separately.

### 3. Start OpenSurge

For **Same-LAN DHCP takeover**, follow **Gateway controls** in Network Settings:

1. Select **保存网络快照与离线恢复卡** to save the network snapshot and offline
   recovery card.
2. Switch the Mac to a fixed IPv4 address.
3. Disable router DHCP when prompted.
4. Return to OpenSurge and run the DHCP OFFER probe.
5. After the probe succeeds, select **启动 OpenSurge** (Start OpenSurge).
6. Reconnect devices to the network to begin using it.

To confirm a particular device is connected correctly, expand **Next: Check device
connectivity**, enter its IPv4 address, and follow the prompts. This optional check
is collapsed by default and does not block everyday use or stopping the gateway.
An unchecked connection is never marked as verified.

Do not quit immediately after disabling router DHCP. OpenSurge keeps the
recovery state active until the network has actually been restored.

## Everyday use

- **总览** (Dashboard) shows gateway state, active devices, and the latest 60
  seconds of traffic trends.
- **来源** (Sources) refreshes subscriptions and shows version differences. A
  refresh creates a draft that still needs to be applied.
- **设备** (Devices) switches local-Mac Rule / Global / Direct and lets each
  downstream device follow gateway rules, use an independent egress, or choose
  IPv4 direct via the main router during DHCP takeover. Those controls do not
  affect each other. The last mode blocks IPv6 egress for that device when
  downstream IPv6 is enabled, although the client may retain SLAAC/RDNSS.
- **策略** (Policies) shows the final composed policies, lets you select members
  of manual groups, and tests proxy latency while the gateway is stopped. While
  it is running, the page manages the actually applied Selectors, and changes
  take effect immediately.
- **连通性** (Connectivity) shows latency, matched rules, and egress chains
  through the applied configuration and current local-Mac mode. It does not
  represent a downstream-device path.
- **连接** (Connections) shows active sessions, destinations, matched rules,
  actual egress chains, and rates by Mac or downstream device. Search, filter
  by protocol or source address family, sort, paginate, or pause the display.
  Dashboard and Devices provide shortcuts.
- **诊断** (Diagnostics) shows recent operations, providers, and redacted logs.

Dashboard traffic trends retain recent samples when you switch pages within the
same window. The Y-axis still adapts to peaks in the last 60 seconds, with labels
showing its current range; a quick page switch does not start the chart from an
empty history. Chart sampling pauses while the window is hidden and resumes when
it becomes visible. Reloading the page or relaunching the app starts a new history.

Connections keeps registered devices even when they have no active traffic,
and distinguishes a DHCP lease, an unapplied registration, and observed traffic.
No active sessions does not mean a device is offline. IPv4 traffic sent directly
to the main router is outside this observation scope. When identity is verified,
IPv4 and downstream IPv6 sessions belong to the same device; uncertain sources
remain unclassified. This is an active-session view. Pausing the display does
not pause networking, and failed updates label the last sample as stale.

The menu bar panel and Web GUI sidebar both provide **合盖保持运行** (Keep
Running with Lid Closed). It is off by default, applies only to the current
OpenSurge run, and is independent of gateway state. Quitting OpenSurge or
rebooting the Mac releases it. Lid-closed operation increases heat and battery
use; never place a running Mac in an unventilated bag.

The local-Mac mode affects only new connections entering OpenSurge through TUN
or the local explicit proxy. The mode switch itself does not rewrite macOS
system-proxy settings or downstream behavior; the optional network compatibility
setting above owns system-proxy coordination. See
[local Mac routing modes](local-mac-routing.md).
Green **即时生效** (Applies immediately) controls switch an already-applied
egress. Changes to device identity, candidates, or rules must be saved and then
applied through a gateway reload.

Proxy health and connectivity tests originate from the gateway Mac. They help
confirm that the proxy configuration works, but they do not replace DHCP, DNS,
and TUN validation from a downstream device.

## Stop and restore the network

For **Same-LAN DHCP takeover**, you can stop takeover from **Gateway controls**
without completing the optional device check first:

1. Select **停止 OpenSurge** (Stop OpenSurge) and confirm.
2. Re-enable router DHCP when prompted.
3. Return to OpenSurge and run the DHCP OFFER probe.
4. Restore automatic DHCP on the Mac, or explicitly keep the static IPv4.
5. Confirm that recovery is complete before quitting OpenSurge.

The menu bar provides two different quit actions:

- **只退出桌面 App** (Quit Desktop App Only) closes the main window and menu bar icon.
  Gateway and background services keep running.
- **退出 OpenSurge** (Quit OpenSurge) is available only after the gateway is
  stopped and no recovery action remains. It quits the desktop app and user
  Control Service.

## Uninstall

Select **卸载 OpenSurge…** (Uninstall OpenSurge) at the bottom of the menu bar
panel. It becomes available whenever the gateway state is **stopped**. A DHCP
takeover recovery reminder or a system IPv4-forwarding setting that was
already enabled does not block uninstall and is not changed by the uninstaller.

The confirmation offers two choices:

- **保留数据并卸载** (Uninstall and Keep Data) removes the app, Control Service,
  and root Helper while preserving configuration, subscription credentials,
  and policy data for reinstallation.
- **彻底卸载** (Uninstall Everything) also deletes configuration, credentials,
  runtime records, and logs.

macOS asks for administrator authorization. If the gateway is running, first
stop it in **网络设置** (Network Settings). Upgrades do not require uninstall;
installing a newer package directly preserves existing data.

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
