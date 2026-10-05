# Mac 本机系统代理协同契约

本页维护默认关闭的 HTTP/HTTPS 兼容层、冲突检查与快照恢复规则。

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

The coordination toggle is an explicit temporary takeover contract. When
reconciling a runtime interrupted by a system reboot, OpenSurge restores the
same startup snapshot unconditionally, just as it does for an ordinary stop or
rollback. HTTP/HTTPS changes made while the takeover was active are therefore
replaced by that snapshot. A restore failure still fails closed and keeps
runtime state retryable.

## 验证入口

命令级测试覆盖解析、写入范围、顺序、回滚和 state 保留。真实 Network Extension 冲突的兼容验收见 [透明代理门槛](../validation/test-gates.md#透明代理门槛)；关闭兼容层的 TUN Lab 仅证明回归范围。
