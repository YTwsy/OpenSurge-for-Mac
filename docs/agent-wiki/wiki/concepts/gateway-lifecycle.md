# 网关生命周期

当任务涉及 gateway startup、shutdown、rollback、runtime state 或服务职责边界
时，先读这个页面。

OpenSurge for Mac 会把宿主 Mac 变成下游 IPv4 LAN gateway。当前 runtime path
协调四类职责：

- dnsmasq 为下游客户端提供 DHCP 和 DNS；
- mihomo 提供代理能力，并在启用时承担透明 TUN 处理；
- macOS pf 负责从下游 LAN 到上游接口的 NAT；
- macOS IPv4 forwarding 由 sysctl 管理，并在停止时恢复。
- 显式启用时，macOS 上游网络服务的 HTTP/HTTPS 系统代理作为 TUN 兼容层，并由
  runtime state 保存启动前快照。

## Start 顺序

`internal/gateway/manager.go` 负责当前顺序。

`start` 会：

1. 要求 root 权限；
2. 如果 runtime state 已存在则拒绝启动；
3. 确保 runtime directories 存在；
4. preflight dnsmasq、mihomo、pf、sysctl、interfaces 和 LAN IP 归属；
5. 写入 mihomo、dnsmasq 和 pf config artifacts；
6. 记录启动前 IPv4 forwarding、PF enabled 状态；若启用本机系统代理协同，同时
   检查现有代理冲突并保存 HTTP/HTTPS 快照；
7. 在修改 host network 前保存 runtime state；
8. 启用 IPv4 forwarding；
9. 启动 mihomo；
10. 最多等待 10 秒让 mihomo 运行时确认 TUN ready；若失败，先给 mihomo 3 秒
    SIGTERM 清理窗口，再按需 SIGKILL 并 rollback；
11. 启动 dnsmasq；
12. 加载 PF anchor；
13. 所有网关服务 ready 后，才把上游 network service 的 HTTP/HTTPS 代理指向本机
    mihomo mixed-port。

Rollback 是 start 契约的一部分。如果系统代理可能已经写入，会先恢复其启动前状态，
再尝试停止已经启动的服务、卸载 PF 状态并恢复 forwarding。系统代理恢复失败时保留
runtime state 和服务，避免 macOS 继续指向已经停止的本机代理端口。

在 `same_wifi_dhcp` 中，gateway start 发生前，恢复状态机已经可能把 Mac 设为
固定 IPv4，并要求操作者关闭路由器 DHCP。gateway rollback 只恢复本次 start
拥有的进程、PF、forwarding 和 runtime state，不能重新开启路由器 DHCP，也不会
在未确认 DHCP server 可用时把 Mac 冒险切回自动 DHCP。

因此固定 IPv4 已应用、但 gateway 尚未 active 时必须提供“放弃 DHCP 接管”：

- 若 DHCP OFFER 已可见，恢复 Mac 自动 DHCP 并完成恢复，菜单栏可退出 OpenSurge；
- 若没有 DHCP OFFER，明确以 `complete_static` 结束：不冒险切换 Mac，保留固定
  IPv4，也不声称路由器 DHCP 或其他客户端自动获取能力已恢复；这是用户主动放弃
  后的终态，因此菜单栏可退出 OpenSurge；
- TUN 启动失败时保留 `router_dhcp_disabled_confirmed` 和失败说明，让操作者选择
  解决冲突后重试，或走上述放弃/恢复分支。

## Stop 顺序

`stop` 会：

1. 要求 root 权限；
2. 如果存在 runtime state，则加载它；
3. 若 runtime state 有系统代理快照，先恢复 HTTP/HTTPS 代理；恢复失败则保留服务和 state；
4. 停止 dnsmasq；
5. 停止 mihomo；
6. 如果 PF anchor 已加载，则卸载 PF anchor；
7. 恢复 IPv4 forwarding 到启动前的值；
8. 移除 runtime state。

Stop 应该能容忍部分 runtime pieces 已经缺失。这个项目会修改 host network，
所以清理质量是正确性的一部分。

若停止任一服务、PF 或 forwarding 恢复失败，manager 会保留 runtime state 与 applied
device-policy snapshot，避免把仍运行或 degraded 的网关误记为已完全停止，并允许后续
重试清理。所有清理步骤仍会尽量执行；只有这一轮清理没有错误时才移除 state。

### 系统重启后的中断状态

root Helper 与 Control Service 会由 launchd 在开机后恢复，但 dnsmasq、mihomo、PF anchor
与 IPv4 forwarding 不会被 runtime state 文件冒充为已经恢复。每次正常 start 都把当前
boot session 和子进程启动指纹写进 state。Status 发现 state 来自上一次开机时，必须报告
`runtime_state=interrupted`，不得根据旧 PID 探测 mihomo API；`reload` 与
`restart-mihomo` 也必须拒绝这份不完整数据面。

对 interrupted runtime 执行 stop 是专门的 reconciliation，不是普通 stop：它不向旧 PID
发送信号，不卸载本次开机的 PF，不改写本次开机的 IPv4 forwarding；若 state 记录了
系统代理临时接管，则与普通 Stop 一样无条件恢复启动前的 HTTP/HTTPS 快照；
接管期间手动修改的 HTTP/HTTPS 设置也会被该快照替换。若恢复失败，则 fail closed 并
保留 state。
最后移除旧 runtime state 与 applied device-policy snapshot，用户随后可显式
重新启动完整网关。旧版本没有 boot session 字段的 state 通过 `started_at` 与本次系统
启动时间比较迁移；没有任何 boot 归属证据的 state 不能被当作当前运行态。

即使在同一次开机内，PID 也可能在 dnsmasq/mihomo 异常退出后复用。新 runtime state
同时保存进程启动指纹；status、stop 与 restart 只有在 PID 和启动指纹都匹配时才把它当作
OpenSurge 子进程。指纹不匹配表示原子进程已经消失，清理不得终止占用该 PID 的其他进程。

## Reload 顺序

`reload` 只接受正在健康运行的网关。它先在同级临时 runtime 中使用同一份 desired 配置
渲染 mihomo、dnsmasq 与 PF artifacts，执行静态检查、接口/LAN IP、protected/reservation
冲突检查和真实 `mihomo -t`。这一步不写 applied snapshot，也不改变 host network。

全部通过后才调用完整 `stop`，再用已经通过校验的同一份 immutable config 调用完整
`start`。成功会自然写入新的 applied device-policy snapshot/digest；若使用 imported
profile，也把 profile 内容 digest 写进 runtime state，作为运行版本的唯一依据。预校验失败保持现有运行态；
stop 失败保留 state；stop 已成功但 start 失败时网关保持 stopped，由 Control API 根据
拓扑进入明确的重试/恢复路径。Reload 不承诺零中断，也不做 mihomo/dnsmasq 热替换。
新 mihomo 进程仍必须通过同一套 TUN readiness，否则 start fail closed 并 rollback。

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
router 或 DNS。若已启用系统代理协同，替代进程失败时先恢复启动前系统代理，避免端点
继续指向已停止的 mihomo；state 保持 Mihomo PID 为 0，便于再次执行恢复或完整
`stop`。旧事故日志不会被新进程清空。Control API 在 same-WiFi DHCP 拓扑中只允许 active、
client validated 或明确跳过客户端验收的接管阶段执行，且成功或失败都不改变 DHCP 恢复
状态机。替代进程必须通过 TUN readiness。

Control Service 会在当前 boot、有效 active runtime 和允许的 DHCP 接管阶段内监测这条
恢复路径。Mihomo 进程缺失立即触发一次自动恢复；controller 的 `connection refused`
需要连续两次观测才触发。每个未恢复 incident 最多自动尝试一次；命令完成后仍要等新的
健康 status 才算恢复，持续异常会转为 `failed` 并在连通性页显示手动兜底。上次开机留下
的 interrupted runtime、配置读取失败和尚未 active 的 DHCP 接管阶段都不会触发。

start、stop、reload、手动/自动 `restart-mihomo` 和运行中 source apply 共享 Control
Service 生命周期互斥锁，避免两个 helper 动作同时修改 runtime。这个自动化只实现了窄的
Mihomo-only 重启边界；在真实 same-WiFi 断开/重连门槛完成前，不得把单元测试描述成物理
链路恢复已经验收。

## same-WiFi 固定 IPv4 确认

恢复状态机第 2 步不能只以 `networksetup -setmanual` 返回成功作为完成证据。Control API
随后必须回读目标网络服务，确认其 IPv4 配置方式为手动，且 IPv4、子网掩码和路由器与
本次目标配置一致；确认失败时返回 `static_ipv4_not_applied`，让 Web GUI 提示操作者检查
macOS“系统设置 → 网络 → 详细信息 → TCP/IP”，并且不得进入 `mac_static`。

## 产品不变量

生命周期服务于全屋代理能力。不要把 DHCP、mihomo、pf 或 forwarding 当作互不
相关的 demo。只有组合后的 LAN path 仍然可理解、可回滚、可验证，gateway 改动
才算正确。

## 验证

用 `make test` 验证代码层行为。宣称真实网关生命周期在 host network 上
工作前，运行 `make lab-test`。涉及透明代理行为时，运行 `make lab-test-tun`。
