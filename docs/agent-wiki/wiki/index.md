# Agent Wiki 索引

这个 wiki 是 OpenSurge for Mac 面向 agent 的上下文层。它从
`../sources/` 中整理稳定知识，并指向仓库内仍然作为事实来源的文件。

当任务涉及产品方向、网关行为、透明代理或验证门槛时，从这里开始。

## 核心上下文

- [网关生命周期](concepts/gateway-lifecycle.md)：Mac 如何成为下游 LAN
  gateway，以及如何停止并恢复。
- [macOS TUN 透明代理](concepts/macos-tun-transparent-proxy.md)：为什么
  TUN 是透明代理主线，以及哪些旧旋钮必须保持 inactive。
- [下游 IPv6 接管](concepts/downstream-ipv6-takeover.md)：独立 LAN、同 LAN DHCP 与
  选择性旁路由的自动/手工接入、无系统 TUN packet ingress、MAC 设备身份和 TCP/UDP
  验收边界。
- [mihomo profile overlay](concepts/mihomo-profile-overlay.md)：如何导入
  mihomo 代理/规则 section，同时保持 OpenSurge 接管网关字段。
- [每设备策略覆盖](concepts/device-policy-overlays.md)：如何以 DHCP reservation 和
  `SRC-IP-CIDR` 在一个 mihomo 进程中实现独立的设备策略。
- [Mac 本机流量模式](concepts/local-mac-routing-modes.md)：如何用 source-scoped
  overlay 实现规则 / 全局 / 直连，同时保持下游设备规则不变。
- [Mac 本机系统代理协同](concepts/local-system-proxy-coordination.md)：默认关闭的
  TUN HTTP/HTTPS 兼容层、fail-closed 冲突检查和恢复契约。
- [GUI 控制面](concepts/gui-control-plane.md)：React Web GUI、SwiftUI 菜单栏
  launcher、本地 API 与恢复状态的职责边界。
- [合盖运行临时接管](../sources/decisions/lid-closed-sleep-prevention.md)：为什么
  `caffeinate` 不满足合盖需求，以及 Helper lease、ownership marker 和恢复边界。
- 许可证边界：OpenSurge 自有代码采用 `GPL-3.0-only`；随 pkg 分发的独立组件保留
  各自许可证与对应源码链接，见根目录 `LICENSE` 和 `THIRD_PARTY_NOTICES.md`。
- [验证门槛](concepts/validation-gates.md)：哪些检查能证明哪些结论。

## 项目形态

OpenSurge for Mac 是一个开源的 Surge for Mac 风格 macOS 网关与控制面。它的
核心能力是全屋代理：Mac 对下游设备承担网关职责，dnsmasq 按拓扑提供 DHCP/DNS，
mihomo 提供代理行为，macOS pf/sysctl 提供 IPv4 NAT 和 forwarding。实验性的
下游 IPv6 通过 dnsmasq RA/SLAAC/RDNSS 或手工 ULA 接入，再由 macOS BPF broker
与本项目补丁构建的 mihomo 用户态数据面接管。

当前面向操作者的主要控制面是 React Web GUI。SwiftUI 菜单栏 App 显示网关状态、
恢复提醒并打开 Web GUI；loopback Control API 连接界面与 Go 业务规则，root Helper
执行固定的特权动作。`omg` CLI 不再代表产品形态，但仍是受支持的运维、诊断、
自动化和恢复接口。

CLI 契约继续优先保持机器可读：`status`、`doctor`、`leases`、`logs`、
`policies`、`local-routing`、`devices`、`connections`、`providers`、`provider-update` 和 `snapshot` 支持 JSON
输出。`logs --tail N --format json` 会返回最近的 dnsmasq/mihomo 日志行，并对每个
日志文件标出存在状态和读取错误。`snapshot --format json` 聚合 status、doctor、
leases、日志尾部、策略组、连接和 provider 状态，并把 mihomo API 不可用记录在局部
字段里，供 GUI 后端与自动化诊断复用。
`start --format json` 和 `stop --format json` 在动作成功后返回结构化成功 payload；
失败仍保留非零退出码，并在 `--format json` 时把
`{"command":"...","ok":false,"error":"..."}` 写到 stderr。

## 事实来源

- 公开范围与 App/CLI 工作流：`README.md`
- 示例配置：`examples/config.example.yaml`
- GUI 控制面：`internal/controlapi/`、`web/`、`apps/menubar/` 和
  `concepts/gui-control-plane.md`
- 生命周期代码：`internal/gateway/manager.go`
- 配置验证：`internal/config/validator.go`
- mihomo profile 导入：`internal/mihomo/profile.go` 和
  `docs/agent-wiki/sources/decisions/mihomo-profile-overlay.md`
- Mac 本机模式：`internal/mihomo/local_routing.go` 和
  `docs/agent-wiki/sources/decisions/local-mac-routing-modes.md`
- Mac 本机系统代理：`internal/macosnetwork/system_proxy.go` 和
  `docs/agent-wiki/sources/decisions/local-system-proxy-coordination.md`
- 下游 IPv6 接管：`internal/ipv6packet/`、`internal/macosipv6/` 和
  `docs/agent-wiki/sources/decisions/downstream-ipv6-takeover.md`
- Virtual LAN lab：`tests/lab/README.md` 和 `tests/lab/lab.sh`
- 真实设备 smoke：`tests/real-device/README.md` 和
  `tests/real-device/smoke.sh`
- 真实设备 smoke 当前进度：
  `docs/agent-wiki/sources/validation/real-device-smoke.md`
- same-LAN TUN smoke：`tests/same-lan/README.md`、
  `tests/same-lan/smoke.sh` 和
  `docs/agent-wiki/sources/validation/same-lan-tun-smoke.md`

当这些事实来源的变化会影响未来 agent 判断时，更新这个 wiki。
