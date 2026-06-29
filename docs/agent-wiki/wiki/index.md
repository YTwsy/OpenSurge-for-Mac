# Agent Wiki 索引

这个 wiki 是 OpenSurge for Mac 面向 agent 的上下文层。它从
`../sources/` 中整理稳定知识，并指向仓库内仍然作为事实来源的文件。

当任务涉及产品方向、网关行为、透明代理或验证门槛时，从这里开始。

## 核心上下文

- [网关生命周期](concepts/gateway-lifecycle.md)：Mac 如何成为下游 LAN
  gateway，以及如何停止并恢复。
- [macOS TUN 透明代理](concepts/macos-tun-transparent-proxy.md)：为什么
  TUN 是透明代理主线，以及哪些旧旋钮必须保持 inactive。
- [验证门槛](concepts/validation-gates.md)：哪些检查能证明哪些结论。

## 项目形态

OpenSurge for Mac 正在走向一个开源的 Surge for Mac 风格 macOS gateway。它的
核心能力是全屋代理：下游设备接入由 Mac 服务的 LAN，从 dnsmasq 获得
DHCP/DNS，并把流量交给 Mac；mihomo 提供代理行为，macOS pf/sysctl 提供 NAT
和 forwarding。

当前仓库是 CLI-driven MVP。把 CLI 当作当前控制面，不要把它误认为最终产品
边界。

## 事实来源

- 公开范围：`README.md`
- 示例配置：`examples/config.example.yaml`
- 生命周期代码：`internal/gateway/manager.go`
- 配置验证：`internal/config/validator.go`
- Virtual LAN lab：`tests/lab/README.md` 和 `tests/lab/lab.sh`

当这些事实来源的变化会影响未来 agent 判断时，更新这个 wiki。
