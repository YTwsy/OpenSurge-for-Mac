# OpenSurge 架构简报

本页维护当前架构、组件职责和实现入口；产品方向与协作约定见 [AGENTS.md](../../../AGENTS.md)，
具体接口、领域规则和验证由相应来源维护。

v0.3 以 Wails + 系统 WebView 承载共享 React 主窗口和菜单栏面板，浏览器 Web GUI 继续可用。
独立的 Go Control Service 协调业务，root Helper 执行固定特权动作，`omg` CLI 用于运维、
诊断、自动化和恢复。主窗口、菜单栏与 CLI 应复用现有 Go 业务规则，不建立平行业务实现。

Mac 为下游承担网关职责：dnsmasq 按拓扑提供 DHCP/DNS，mihomo 承担代理能力，macOS
pf/sysctl 管理 IPv4 NAT 与 forwarding。OpenSurge 接纳代理与规则来源，同时拥有网关配置
和恢复责任。Mac 本机与 IPv4 下游透明流量使用 TUN；实验性的下游 IPv6 通过 BPF broker
与 patched Mihomo packet path 接入，并保留设备路由身份。

## 实现入口

- 产品范围与用户操作：[用户文档](../../user/README.md)。
- 当前默认值：`examples/config.example.yaml`。
- 接管与恢复：`internal/gateway/manager.go`、[生命周期](decisions/gateway-lifecycle.md)。
- API 与桌面：`internal/controlapi/`、`web/`、`apps/desktop/`、[Control API](control-api.md)。
- 配置合成：`internal/mihomo/`、[profile overlay](decisions/mihomo-profile-overlay.md)。
- IPv6：`internal/ipv6packet/`、`internal/macosipv6/`、[IPv6 契约](decisions/downstream-ipv6-takeover.md)。
- 验证方法：[test-gates](validation/test-gates.md)；已有结果：[evidence-map](validation/evidence-map.md)。
