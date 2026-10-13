# 局域网 DHCP 接管恢复契约

本页维护 `same_wifi_dhcp` 的持久恢复阶段、主动证据和用户确认。网关进程与资源清理由 [生命周期契约](gateway-lifecycle.md) 维护；用户步骤见 [App 指南](../../../user/app-user-guide.zh-CN.md)。

## 状态与启动证据

`/api/v1/recovery` 持久化以下受验证状态机：

```text
idle -> prepared -> mac_static -> router_dhcp_disabled_confirmed
     -> gateway_active [-> client_validated | client_validation_skipped]
     -> gateway_stopped_waiting_router_dhcp
        -> router_dhcp_restored -> complete | complete_static
        -> complete_static
```

局域网 DHCP 接管的恢复状态、source snapshots 和 operation records 保存在用户的
`~/Library/Application Support/OpenSurge/`。`same_wifi_dhcp` start 需要持久化的路由器
DHCP 已关闭确认；正常确认与恢复由 root helper 的 DHCP OFFER 探测提供证据。stop 后仍
保持恢复警报，直到路由器 DHCP 与 Mac 自动获取恢复，或用户明确选择保留静态 IPv4 并
结束流程。停止后的恢复动作不能被 takeover
plan 的 LAN IP、router 等启动期 blocker 禁用；这些 blocker 只约束启动前阶段。若主动
OFFER 探测不可用，认证后的 Web GUI 提供带断网警告和显式人工确认的兜底：跳过 OFFER
证据并真实执行 Mac 自动 DHCP 恢复，然后才写入 `complete`，不能只清除 recovery 标记。
如果用户明确选择长期保持静态 IPv4，网关成功停止后也可跳过路由器 DHCP 探测与 Mac
自动 DHCP 恢复，直接进入 `complete_static`。该动作不调用 `ProbeDHCP` 或 `SetDHCP`，必须
保留持久化说明，并提示其他客户端需要有效静态配置或另一个 DHCP 服务器。

确认路由器 DHCP 已关闭时，root Helper 主动发送 DHCPDISCOVER；仍收到任何 OFFER 就拒绝确认。状态机不自动修改未知路由器，同一二层 LAN 也不提供不可绕过隔离。

## 客户端验收与恢复卡

局域网 DHCP 接管 start 后还有 `client_validated` 阶段：要求 active lease、DHCPACK、客户端源 IP
DNS 与 mihomo TUN 日志，并保存用户对网关/DNS、无显式代理和 IPv6 绕过警告的确认。
网络页将这项检查放在默认折叠的“下一步：检查设备接入”中，作为可选操作；运行控制
直接提供 stop，不要求先检查或跳过。只有检查成功才进入 `client_validated`，未执行检查
不能显示或对外宣称已经验收。API 保留显式 `client_validation_skipped` 及“没有客户端
路径证据”的记录以兼容已有调用；界面不再提供单独的跳过步骤。三种运行阶段都可直接
stop，不能让检查失败阻塞网络恢复。接管与恢复步骤在运行期间默认折叠，启动前及停止后
默认展开；停止后的 DHCP 与 Mac 网络恢复提示始终直接显示。

`prepared` 只表示恢复网络快照和离线恢复卡已经落盘：此时 Mac、路由器和 DHCP 尚未
改变。它不是跨页面高风险告警；`gateway_active` / `client_validated` 是预期的稳定接管
状态，显示运行/验收信息而不是“恢复尚未完成”。跨页面恢复告警只用于接管启动前已经
改变网络但尚未运行的阶段，以及 `gateway_stopped_waiting_router_dhcp` /
`router_dhcp_restored` 等停止后的恢复阶段。预备阶段允许修正并保存 desired 网络配置；保存会清除预备
恢复卡并回到第 1 步，避免用户用未保存的 topology 或 LAN IPv4 执行第 2 步。准备恢复卡
之前必须拿 configured `gateway.lan_ip` 与实时路由器/掩码做同网段校验，失败不得写入
`prepared`。菜单栏从恢复入口应打开 Web GUI 的 `network` 页面，而不是不存在的
`recovery` 路径。

Mac 执行 `networksetup -setdhcp` 后，DHCP 租约与 router 字段可能短暂为空。恢复动作成功
后不要立即重新运行 takeover plan 的完整 IPv4 discovery，否则会把正常续租窗口误报成
`does not expose a complete IPv4 configuration`；后续页面刷新再做常规发现。

网络页必须直接显示持久化 `network_snapshot` 中的原始 IPv4、路由器、DNS、网络服务、
接口与掩码，并通过受认证的 `GET /api/v1/recovery/card` 提供中文恢复卡查看与下载。
`prepared` 阶段允许调用 `POST /api/v1/recovery/discard` 销毁快照和离线卡并回到 `idle`；
一旦进入 `mac_static`，这条无网络动作的捷径必须硬拒绝。停止后的人工兜底只能从
`gateway_stopped_waiting_router_dhcp` 进入，并且必须调用 privileged `SetDHCP` 成功后才
写入 `complete`。独立的 `keep-static` 动作只允许从
`gateway_stopped_waiting_router_dhcp` / `router_dhcp_restored` 进入 `complete_static`；它不
冒充 DHCP 恢复，也不触发任何网络 runner。

路由器地址为有效 IPv4 时，界面提供 HTTP 管理页链接；关闭和恢复 DHCP 阶段显示通用 LAN / 网络设置 / DHCP 服务器路径，以及无法自动发现路由器时的 fallback。

## 固定 IPv4 的回读确认

恢复状态机第 2 步不能只以 `networksetup -setmanual` 返回成功作为完成证据。Control API
随后必须回读目标网络服务，确认其 IPv4 配置方式为手动，且 IPv4、子网掩码和路由器与
本次目标配置一致；确认失败时返回 `static_ipv4_not_applied`，让 Web GUI 提示操作者检查
macOS“系统设置 → 网络 → 详细信息 → TCP/IP”，并且不得进入 `mac_static`。

## 启动失败后的放弃路径

固定 IPv4 已应用而网关尚未 active 时，必须提供“放弃 DHCP 接管”：已观察到 DHCP OFFER 时恢复自动 DHCP；无 OFFER 时可由用户明确保留静态 IPv4，以 `complete_static` 结束。TUN 启动失败保留 `router_dhcp_disabled_confirmed` 与错误供重试，不能冒险切回无 DHCP 服务的网络。

## 重载与恢复阶段

DHCP 接管期间成功 reload 保留原有 `gateway_active`、`client_validated` 或
`client_validation_skipped` 阶段。stop 失败保留原 recovery；stop 成功但重新启动失败时
降回 `router_dhcp_disabled_confirmed`，供用户重试启动或进入恢复。候选预检失败不执行
stop，也不改变恢复阶段；具体重载顺序见 [生命周期契约](gateway-lifecycle.md)。

## 验证边界

`client_validation_skipped` 和 `complete_static` 是明确的流程选择，不构成客户端验收或自动获取恢复证据。真实同 LAN 方法见 [物理网络验收](../validation/physical-network.md)，历史结果见 [证据入口](../validation/evidence-map.md#物理网络)，物理上游断链见 [恢复门槛](../validation/test-gates.md#same-WiFi-上游断链恢复门槛)。
