# GUI 控制面

OpenSurge 的完整 GUI 是 `web/` 中的 React 应用，菜单栏 App 是
`apps/menubar/` 中的只读 SwiftUI launcher。两者都只访问
`cmd/opensurge-control` 提供的 loopback API；业务规则继续位于 Go gateway、device、
mihomo 和 runtime 包中。

菜单栏 App 不提供 start/stop 或策略切换。它只消费 `/api/v1/menubar`，显示网关、
客户端、drift 和恢复状态，并通过一次性 bootstrap URL 打开 Web GUI。不要把菜单栏
演变成第二控制面。

菜单栏打开 Web GUI 时先调用 `NSWorkspace.shared.open`，检查其返回值；失败后回退到
`/usr/bin/open`，并在窗口内显示错误。不要把一次性 bootstrap URL 写入错误信息或长期
日志。

Control API 的 bootstrap `expires_at` 来自 Go `time.Time`，可能包含 RFC3339 小数秒；
菜单栏客户端必须同时接受带小数秒和不带小数秒的时间格式，不能把该解码失败误判为浏览器
打开失败。

same-WiFi 恢复状态、source snapshots 和 operation records 保存在用户的
`~/Library/Application Support/OpenSurge/`。`same_wifi_dhcp` start 需要持久化的路由器
DHCP 已关闭确认；确认与恢复均由 root helper 的 DHCP OFFER 探测提供证据。stop 后仍
保持恢复警报，直到路由器 DHCP 与 Mac 自动获取恢复。订阅完整 URL 存在 Keychain，
sources JSON 只保留脱敏 origin。

same-WiFi start 后还有 `client_validated` 阶段：要求 active lease、DHCPACK、客户端源 IP
DNS 与 mihomo TUN 日志，并保存用户对网关/DNS、无显式代理和 IPv6 绕过警告的确认。
紧急 stop 仍允许直接执行，以免验收失败阻塞网络恢复。

网络配置通过 revisioned `GET/PUT /api/v1/config` 修改；只允许 topology、DHCP/DNS、
TUN 和 device-policy 初始化字段，运行中或 recovery required 时拒绝。所有 production
写入经 helper 落到 root-owned config。`/events` 发送真实 config/gateway/drift/recovery
变化，诊断接口返回连接与脱敏后的短日志尾部。

生产 pkg 使用固定 `/` install location 和不可 relocatable 的菜单栏 bundle，把 App 安装
到 `/Applications/OpenSurge Menu Bar.app`；否则 macOS Installer 可能把它 relocate 回
构建工作区的 `payload/Applications`。生产 pkg 把 applied config、mihomo/dnsmasq、runtime 和 helper 放在 root-owned 的
`/Library/Application Support/OpenSurge` / `PrivilegedHelperTools` 下；用户级 Control
Service 只通过 admin 组只读访问 applied 状态，通过 helper 执行固定 privileged 动作。

开发期用 `make web-build`、`make menubar-build` 和 `make test`。这些检查不证明真实
DHCP、TUN 或 per-device 数据面；网络声明仍服从 validation-gates 页面。
