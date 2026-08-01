# OpenSurge for Mac FAQ

This page records common v0.1.24 questions about local-Mac connectivity, TUN
startup, device identity, and per-device egress. Follow the
[OpenSurge for Mac App User Guide](app-user-guide.md) for the complete install
and DHCP recovery workflow.

## Downstream devices work, but the Mac itself has no network access

If downstream DHCP, DNS, and transparent proxying work while the Mac has DNS or
connectivity failures, check for SafeDNS, DNS Proxy, content filtering, or
another Network Extension. Such software can disrupt only the Mac-local TUN DNS
path, so downstream success does not prove that local traffic is healthy.

After safely stopping the gateway and completing any required DHCP recovery,
open **Network Settings → Desired Network Configuration**, keep **mihomo TUN**
enabled, turn on **Mac local system-proxy coordination**, and save. On the next
start, OpenSurge points the current upstream network service's HTTP and HTTPS
proxy settings at the local mihomo mixed-port. Stop, startup rollback, or a
failed mihomo restart restores the pre-start settings.

This option is off by default. It affects only Mac applications that honor
system proxy settings, does not replace TUN, and does not change downstream
devices. OpenSurge refuses to start if HTTP/HTTPS proxying, PAC, or proxy
auto-discovery is already enabled, rather than overwriting an existing setup.
Identify the owner of those settings before changing them.

## TUN startup reports a conflict

During a real start, OpenSurge waits for mihomo to report TUN ready. If readiness
fails, it stops the attempted start, rolls back gateway state it owns, and tries
to include the conflicting route's interface or gateway in the error. A common
cause is another VPN, proxy, or Network Extension whose `utun` already owns the
default route.

Use the reported evidence to stop the conflicting global TUN or VPN, then retry.
Do not delete an interface merely because a `utun` exists, and do not run two
TUNs that both require the public default route. If the failure remains, collect
the current status, routes, and mihomo log from **Diagnostics**.

## Why is a device MAC unavailable, and what happens when switching to DHCP?

In same-LAN manual-gateway mode, OpenSurge supplements current traffic with the
macOS neighbor table on a best-effort basis. A device that has not communicated
with the Mac, an expired neighbor entry, client isolation, Proxy ARP, or
centralized forwarding can leave only an IPv4 observation. This is not by itself
proof of a macOS or OpenSurge regression.

Manual-gateway mode permits registration by fixed IPv4 alone, with MAC as
optional identity metadata. The main router must keep that IPv4 stable and must
not assign it to another device. When switching to DHCP takeover:

- If every registration already has a MAC, the switch proceeds without a
  migration dialog.
- If a unique current MAC is available for an IP-only device, the dialog shows
  it for confirmation before saving and switching.
- If a MAC remains unavailable, the dialog explains that the affected policies
  will pause in DHCP mode. You can inspect devices, switch while pausing those
  policies, or cancel.

Device records, Profiles, and rules are preserved. DHCP mode does not guess
identity from the old IPv4. Once the real MAC is known, open
**Devices → Register a device**, use the original fixed IPv4 to add that MAC,
save, and reload. Never invent a MAC address.

## How do I give each device an independent egress?

1. While the gateway is stopped and configuration is editable, open
   **Network Settings → Desired Network Configuration**, enable
   **Per-device policies**, and save.
2. Open **Devices → Register a device** and select a DHCP lease or currently
   observed device, or enter it manually.
3. Set **Device routing mode** to **Independent device egress** and choose the
   permitted egress candidates.
4. Save the device configuration and start or reload the gateway so its selector
   and rules become applied.
5. After application, use that device's egress selector on the Devices page to
   switch candidates. Selector changes normally affect new connections
   immediately and do not require changing the local-Mac routing mode.

**Follow gateway rules** continues to use the imported or managed global rules.
**Independent device egress** prioritizes the device's own public egress while
keeping LAN and private destinations direct. The local Mac's
**Rule / Fixed egress / Direct** control does not change downstream policies.
