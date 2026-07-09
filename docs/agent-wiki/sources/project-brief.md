---
title: OpenSurge project brief
kind: source
status: seed
---

# OpenSurge 项目简报

OpenSurge for Mac 的目标，是成为一个开源的 Surge for Mac 风格 macOS 网关
与控制面。当前实现是 CLI MVP，但项目方向不是“命令行代理包装器”，而是更
完整的 Mac-native 全屋代理网关。

核心功能是全屋代理：

- Mac 作为下游设备的 IPv4 LAN gateway；
- dnsmasq 在下游 LAN 上提供 DHCP 和 DNS；
- mihomo 是当前代理引擎；
- OpenSurge 可以导入 mihomo 的代理/规则 profile section，但仍由 OpenSurge
  覆盖并接管 LAN 绑定、DNS、TUN 和 API 等网关字段；
- macOS pf 提供 NAT；
- macOS IPv4 forwarding 由 sysctl 管理，并在停止时恢复；
- macOS 透明代理当前通过 mihomo TUN 实现。

实现必须保持可审计、可回滚、可验证。高风险网络行为要先在隔离的 virtual
LAN lab 中验证，再进入普通 LAN 场景。

当前 CLI 是控制面 MVP。`status`、`doctor`、`leases`、`logs`、`policies`、
`connections` 和 `snapshot` 都有机器可读 JSON 形态；`logs --tail N --format
json` 会返回最近的 dnsmasq/mihomo 日志行，并对每个日志文件标出存在状态和读取
错误。`snapshot --format json` 聚合 status、doctor、leases、日志尾部、策略组和
连接，并把 mihomo API 不可用记录在局部字段里，供未来轻 UI 或菜单栏诊断复用。
`start --format json` 和 `stop --format json` 在动作成功后返回结构化成功 payload；
失败仍通过非零退出码和 stderr 表达。

## 当前事实来源

- `README.md` 描述公开产品范围和 CLI 工作流。
- `examples/config.example.yaml` 记录当前配置默认值。
- `internal/gateway/manager.go` 负责 start、rollback 与 stop 顺序。
- `internal/config/validator.go` 约束不受支持的 redir/PF redirect 路径。
- `tests/lab/README.md` 描述 virtual LAN lab 和验证门槛。

## 维护规则

当产品方向、核心网关模型或主要事实来源变化时，更新这个 source。
