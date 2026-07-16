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

若停止任一服务、PF 或 forwarding 恢复失败，manager 会保留 runtime state 与 applied
device-policy snapshot，避免把仍运行或 degraded 的网关误记为已完全停止，并允许后续
重试清理。所有清理步骤仍会尽量执行；只有这一轮清理没有错误时才移除 state。

## Reload 顺序

`reload` 只接受正在健康运行的网关。它先在同级临时 runtime 中使用同一份 desired 配置
渲染 mihomo、dnsmasq 与 PF artifacts，执行静态检查、接口/LAN IP、protected/reservation
冲突检查和真实 `mihomo -t`。这一步不写 applied snapshot，也不改变 host network。

全部通过后才调用完整 `stop`，再用已经通过校验的同一份 immutable config 调用完整
`start`。成功会自然写入新的 applied device-policy snapshot/digest；若使用 imported
profile，也把 profile 内容 digest 写进 runtime state，作为运行版本的唯一依据。预校验失败保持现有运行态；
stop 失败保留 state；stop 已成功但 start 失败时网关保持 stopped，由 Control API 根据
拓扑进入明确的重试/恢复路径。Reload 不承诺零中断，也不做 mihomo/dnsmasq 热替换。

运行中应用 imported profile 额外包一层 config 事务：先保留旧 config，写入并验证新
desired config，再调用上述 reload。失败时恢复旧 config；如果 reload 已完成 stop 且
runtime state 不存在，则尝试用旧 config 重新 start。只有新 start 成功、runtime state
记录新 profile digest 且 `runtime/mihomo.yaml` 已重新生成后，控制面才可把来源标记为
applied。网关停止时应用 profile 只更新 desired，留待下次正常 start。

## Mihomo 独立恢复

`restart-mihomo` 用于上游接口断开并重新关联后，Mihomo/TUN 进程仍存活但出站 socket
没有恢复，或 Mihomo 进程已经退出而网关 runtime 仍存在的场景。它不是配置 reload：

1. 要求 root 权限和已有 gateway runtime state；
2. 对当前已经生成的 applied Mihomo config 运行真实 `mihomo -t`；
3. 先把 runtime state 中的 Mihomo PID 清零，再停止旧进程；
4. 把旧 `mihomo.log` 归档为带 UTC 时间戳的 `mihomo-before-restart-*.log`；
5. 使用同一份 applied config 启动 Mihomo，并原子写回新 PID。

这个动作不停止 dnsmasq、不卸载 PF、不恢复 IPv4 forwarding，也不修改 Mac 静态地址、
router 或 DNS。启动替代进程失败时 state 保持 Mihomo PID 为 0，便于再次执行恢复或完整
`stop`；旧事故日志不会被新进程清空。Control API 在 same-WiFi DHCP 拓扑中只允许 active、
client validated 或明确跳过客户端验收的接管阶段执行，且成功或失败都不改变 DHCP 恢复
状态机。

这是一条显式恢复路径，不是自动 watchdog。只有真实 same-WiFi 链路断开/重连门槛证明
触发条件不会误判、恢复后本机 DIRECT 和代理出口均重新可用，才应增加自动触发。

## 产品不变量

生命周期服务于全屋代理能力。不要把 DHCP、mihomo、pf 或 forwarding 当作互不
相关的 demo。只有组合后的 LAN path 仍然可理解、可回滚、可验证，gateway 改动
才算正确。

## 验证

用 `make test` 验证代码层行为。宣称真实网关生命周期在 host network 上
工作前，运行 `make lab-test`。涉及透明代理行为时，运行 `make lab-test-tun`。
