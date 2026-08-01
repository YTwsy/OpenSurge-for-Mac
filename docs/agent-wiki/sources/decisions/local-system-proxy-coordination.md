# Decision: local system-proxy coordination is an opt-in TUN compatibility layer

Status: accepted

OpenSurge may coordinate the gateway Mac's HTTP and HTTPS system-proxy settings
only when `local_system_proxy.enabled: true` and `transparent.mode: "tun"`.
The default remains disabled. This is a compatibility layer for local apps that
honor macOS system proxies when SafeDNS, DNS Proxy, content filters, or other
Network Extensions interfere with a TUN-only local DNS path. It is not a
replacement for TUN and does not affect downstream devices.

The target network service is resolved from `gateway.upstream_interface`. The
HTTP and HTTPS endpoints use `127.0.0.1:<mihomo.mixed_port>`. OpenSurge does not
write SOCKS, PAC, proxy auto-discovery, or bypass-domain settings.

Before any host-network mutation, startup reads the HTTP/HTTPS, PAC, and
auto-discovery state and persists an HTTP/HTTPS snapshot in runtime state.
Startup fails closed if HTTP or HTTPS proxying is already active, PAC or
auto-discovery is enabled, or either proxy is authenticated. Credentials are
not readable through `networksetup`, so authenticated settings cannot be
safely restored.

The endpoints are enabled only after mihomo/TUN, dnsmasq, PF, and forwarding
are ready. Stop restores the snapshot before stopping mihomo or dnsmasq. A
startup rollback or failed `restart-mihomo` also restores the snapshot first.
If restoration fails, runtime state is retained and gateway services are not
intentionally stopped, avoiding a system proxy that points at a dead listener.

Command-level tests can prove parsing, write scope, ordering, rollback, and
state retention. Existing TUN labs with the setting disabled prove regression
coverage only. Product-level resolution of a Network Extension conflict still
requires a real Mac test with the conflicting extension active, plus start,
traffic, stop, and settings-restoration evidence.
