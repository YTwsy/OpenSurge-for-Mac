# GUI 控制面

OpenSurge 的完整 GUI 是 `web/` 中的 React 应用，菜单栏 App 是
`apps/menubar/` 中的只读 SwiftUI launcher。两者都只访问
`cmd/opensurge-control` 提供的 loopback API；业务规则继续位于 Go gateway、device、
mihomo 和 runtime 包中。

菜单栏 App 不提供 start/stop 或策略切换。它只消费 `/api/v1/menubar`，显示网关、
客户端、drift 和恢复状态，并通过一次性 bootstrap URL 打开 Web GUI。不要把菜单栏
演变成第二控制面。

same-WiFi 恢复状态、source snapshots 和 operation records 保存在用户的
`~/Library/Application Support/OpenSurge/`。`same_wifi_dhcp` start 需要持久化的路由器
DHCP 已关闭确认；确认与恢复均由 root helper 的 DHCP OFFER 探测提供证据。stop 后仍
保持恢复警报，直到路由器 DHCP 与 Mac 自动获取恢复。订阅完整 URL 存在 Keychain，
sources JSON 只保留脱敏 origin。

网络配置通过 revisioned `GET/PUT /api/v1/config` 修改；只允许 topology、DHCP/DNS、
TUN 和 device-policy 初始化字段，运行中或 recovery required 时拒绝。所有 production
写入经 helper 落到 root-owned config。`/events` 发送真实 config/gateway/drift/recovery
变化，诊断接口返回连接与脱敏后的短日志尾部。

生产 pkg 把 applied config、mihomo/dnsmasq、runtime 和 helper 放在 root-owned 的
`/Library/Application Support/OpenSurge` / `PrivilegedHelperTools` 下；用户级 Control
Service 只通过 admin 组只读访问 applied 状态，通过 helper 执行固定 privileged 动作。

开发期用 `make web-build`、`make menubar-build` 和 `make test`。这些检查不证明真实
DHCP、TUN 或 per-device 数据面；网络声明仍服从 validation-gates 页面。
