---
title: OpenSurge project brief
kind: source
status: seed
---

# OpenSurge 项目简报

OpenSurge for Mac 是一个开源的 Surge for Mac 风格 macOS 网关与控制面。
当前实现已经包含本地 Control Service、React Web GUI、SwiftUI
菜单栏 App、root Helper 和 pkg 安装/恢复流程；它不是“命令行代理包装器”，
而是一套 Mac-native 全屋代理网关。

核心功能是全屋代理：

- Mac 作为下游设备的 LAN gateway；
- dnsmasq 按拓扑在下游 LAN 上提供 DHCP、DNS，以及实验性的 IPv6
  RA/SLAAC/RDNSS；
- mihomo 是当前代理引擎；
- OpenSurge 可以导入 mihomo 的代理/规则 profile section，但仍由 OpenSurge
  覆盖并接管 LAN 绑定、DNS、TUN 和 API 等网关字段；
- macOS pf 提供 NAT；
- macOS IPv4 forwarding 由 sysctl 管理，并在停止时恢复；
- Mac 本机与 IPv4 下游透明代理通过 mihomo TUN 实现；
- 实验性的下游 IPv6 ingress 不经过 macOS 系统 TUN，而由 macOS BPF broker
  送入本项目补丁构建的 mihomo `opensurge-packet`/gVisor 数据面，并保留 MAC
  设备身份用于独立策略。

实现必须保持可审计、可回滚、可验证。高风险网络行为要先在隔离的 virtual
LAN lab 中验证，再进入普通 LAN 场景。

面向操作者的主要控制面是 React Web GUI；SwiftUI 菜单栏 App 显示状态与恢复
提醒并打开 Web GUI，不建立第二套网关控制逻辑。两者通过 loopback Control API
复用 Go gateway、device、mihomo 和 runtime 包中的业务规则，固定特权动作由 root
Helper 执行。

`omg` CLI 仍是受支持的运维、诊断、自动化和恢复接口。`status`、`doctor`、
`leases`、`logs`、`policies`、`local-routing`、`devices`、`connections`、
`providers`、`provider-update` 和 `snapshot` 都有机器可读 JSON 形态；
`logs --tail N --format json` 会返回最近的 dnsmasq/mihomo 日志行，并对每个日志
文件标出存在状态和读取错误。`snapshot --format json` 聚合 status、doctor、
leases、日志尾部、策略组、连接和 provider 状态，并把 mihomo API 不可用记录在
局部字段里，供 GUI 后端与自动化诊断复用。
`start --format json` 和 `stop --format json` 在动作成功后返回结构化成功 payload；
失败仍保留非零退出码，并在 `--format json` 时把
`{"command":"...","ok":false,"error":"..."}` 写到 stderr。

## 当前事实来源

- `README.md` 描述公开产品范围和 App/CLI 工作流。
- `examples/config.example.yaml` 记录当前配置默认值。
- `internal/gateway/manager.go` 负责 start、rollback 与 stop 顺序。
- `internal/controlapi/`、`web/`、`apps/menubar/` 和
  `docs/agent-wiki/wiki/concepts/gui-control-plane.md` 记录当前 GUI 控制面边界。
- `internal/config/validator.go` 约束不受支持的 redir/PF redirect 路径。
- `internal/ipv6packet/`、`internal/macosipv6/` 和
  `docs/agent-wiki/sources/decisions/downstream-ipv6-takeover.md` 记录实验性的
  下游 IPv6 路径。
- `tests/lab/README.md` 描述 virtual LAN lab 和验证门槛。

## 维护规则

当产品方向、核心网关模型或主要事实来源变化时，更新这个 source。
