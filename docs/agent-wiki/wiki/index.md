# Agent Wiki 索引

这个 wiki 是 OpenSurge for Mac 面向 agent 的上下文层。它从
`../sources/` 中整理稳定知识，并指向仓库内仍然作为事实来源的文件。

当任务涉及产品方向、网关行为、透明代理或验证门槛时，从这里开始。

## 核心上下文

- [网关生命周期](concepts/gateway-lifecycle.md)：Mac 如何成为下游 LAN
  gateway，以及如何停止并恢复。
- [macOS TUN 透明代理](concepts/macos-tun-transparent-proxy.md)：为什么
  TUN 是透明代理主线，以及哪些旧旋钮必须保持 inactive。
- [mihomo profile overlay](concepts/mihomo-profile-overlay.md)：如何导入
  mihomo 代理/规则 section，同时保持 OpenSurge 接管网关字段。
- [每设备策略覆盖](concepts/device-policy-overlays.md)：如何以 DHCP reservation 和
  `SRC-IP-CIDR` 在一个 mihomo 进程中实现独立的设备策略。
- [GUI 控制面](concepts/gui-control-plane.md)：React Web GUI、SwiftUI 菜单栏
  launcher、本地 API 与恢复状态的职责边界。
- 许可证边界：OpenSurge 自有代码采用 `GPL-3.0-only`；随 pkg 分发的独立组件保留
  各自许可证与对应源码链接，见根目录 `LICENSE` 和 `THIRD_PARTY_NOTICES.md`。
- [验证门槛](concepts/validation-gates.md)：哪些检查能证明哪些结论。

## 项目形态

OpenSurge for Mac 正在走向一个开源的 Surge for Mac 风格 macOS gateway。它的
核心能力是全屋代理：下游设备接入由 Mac 服务的 LAN，从 dnsmasq 获得
DHCP/DNS，并把流量交给 Mac；mihomo 提供代理行为，macOS pf/sysctl 提供 NAT
和 forwarding。

当前仓库是 CLI-driven MVP。把 CLI 当作当前控制面，不要把它误认为最终产品
边界。

当前控制面契约优先保持机器可读：`status`、`doctor`、`leases`、`logs`、
`policies`、`devices`、`connections`、`providers`、`provider-update` 和 `snapshot` 支持 JSON
输出。`logs --tail N --format json` 会返回最近的 dnsmasq/mihomo 日志行，并对每个
日志文件标出存在状态和读取错误。`snapshot --format json` 聚合 status、doctor、
leases、日志尾部、策略组、连接和 provider 状态，并把 mihomo API 不可用记录在局部
字段里，适合后续轻 UI 或菜单栏诊断界面复用。
`start --format json` 和 `stop --format json` 在动作成功后返回结构化成功 payload；
失败仍保留非零退出码，并在 `--format json` 时把
`{"command":"...","ok":false,"error":"..."}` 写到 stderr。

## 事实来源

- 公开范围：`README.md`
- 示例配置：`examples/config.example.yaml`
- 生命周期代码：`internal/gateway/manager.go`
- 配置验证：`internal/config/validator.go`
- mihomo profile 导入：`internal/mihomo/profile.go` 和
  `docs/agent-wiki/sources/decisions/mihomo-profile-overlay.md`
- Virtual LAN lab：`tests/lab/README.md` 和 `tests/lab/lab.sh`
- 真实设备 smoke：`tests/real-device/README.md` 和
  `tests/real-device/smoke.sh`
- 真实设备 smoke 当前进度：
  `docs/agent-wiki/sources/validation/real-device-smoke.md`
- same-LAN TUN smoke：`tests/same-lan/README.md`、
  `tests/same-lan/smoke.sh` 和
  `docs/agent-wiki/sources/validation/same-lan-tun-smoke.md`

当这些事实来源的变化会影响未来 agent 判断时，更新这个 wiki。
