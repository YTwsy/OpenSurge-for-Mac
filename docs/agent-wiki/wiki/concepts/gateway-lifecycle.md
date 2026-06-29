# 网关生命周期

当任务涉及 gateway startup、shutdown、rollback、runtime state 或服务职责边界
时，先读这个页面。

OpenSurge for Mac 会把宿主 Mac 变成下游 IPv4 LAN gateway。当前 runtime path
协调四类职责：

- dnsmasq 为下游客户端提供 DHCP 和 DNS；
- mihomo 提供代理能力，并在启用时承担透明 TUN 处理；
- macOS pf 负责从下游 LAN 到上游接口的 NAT；
- macOS IPv4 forwarding 由 sysctl 管理，并在停止时恢复。

## Start 顺序

`internal/gateway/manager.go` 负责当前顺序。

`start` 会：

1. 要求 root 权限；
2. 如果 runtime state 已存在则拒绝启动；
3. 确保 runtime directories 存在；
4. preflight dnsmasq、mihomo、pf、sysctl、interfaces 和 LAN IP 归属；
5. 写入 mihomo、dnsmasq 和 pf config artifacts；
6. 记录启动前 IPv4 forwarding 状态和 PF enabled 状态；
7. 在修改 host network 前保存 runtime state；
8. 启用 IPv4 forwarding；
9. 启动 mihomo；
10. 启动 dnsmasq；
11. 加载 PF anchor。

Rollback 是 start 契约的一部分。如果后续步骤失败，manager 会尝试停止已经
启动的服务、卸载 PF 状态，并恢复 forwarding。

## Stop 顺序

`stop` 会：

1. 要求 root 权限；
2. 如果存在 runtime state，则加载它；
3. 停止 dnsmasq；
4. 停止 mihomo；
5. 如果 PF anchor 已加载，则卸载 PF anchor；
6. 恢复 IPv4 forwarding 到启动前的值；
7. 移除 runtime state。

Stop 应该能容忍部分 runtime pieces 已经缺失。这个项目会修改 host network，
所以清理质量是正确性的一部分。

## 产品不变量

生命周期服务于全屋代理能力。不要把 DHCP、mihomo、pf 或 forwarding 当作互不
相关的 demo。只有组合后的 LAN path 仍然可理解、可回滚、可验证，gateway 改动
才算正确。

## 验证

用 `make test` 验证代码层行为。宣称真实网关生命周期在 host network 上
工作前，运行 `make lab-test`。涉及透明代理行为时，运行 `make lab-test-tun`。
