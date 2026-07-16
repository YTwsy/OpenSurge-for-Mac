# GUI 控制面

OpenSurge 的完整 GUI 是 `web/` 中的 React 应用，菜单栏 App 是
`apps/menubar/` 中的只读 SwiftUI launcher。两者都只访问
`cmd/opensurge-control` 提供的 loopback API；业务规则继续位于 Go gateway、device、
mihomo 和 runtime 包中。

菜单栏 App 不提供 start/stop 或策略切换。它只消费 `/api/v1/menubar`，显示网关、
客户端、drift 和恢复状态，并通过一次性 bootstrap URL 打开 Web GUI。不要把菜单栏
演变成第二控制面。

菜单栏 indicator 先判断需要用户处理的 recovery，再判断 gateway 是否明确 `stopped`；
只有正在运行或 degraded 的 gateway 才把 drift/doctor failure 表示为“运行异常”。停止状态
下的 runtime doctor failure 或待应用配置不能覆盖“OpenSurge 网关已停止”。

“退出菜单栏 App”只终止菜单栏图标，不会停止用户级 Control Service，也不会停止正在
运行的 DHCP/DNS、mihomo、PF 或 forwarding。该按钮必须先显示状态感知的二次确认；
网关运行时应明确列出仍会继续的服务，状态不可达时应提示先检查，而不是暗示后台已退出。

菜单栏打开 Web GUI 时先调用 `NSWorkspace.shared.open`，检查其返回值；失败后回退到
`/usr/bin/open`，并在窗口内显示错误。不要把一次性 bootstrap URL 写入错误信息或长期
日志。

Control API 的 bootstrap `expires_at` 来自 Go `time.Time`，可能包含 RFC3339 小数秒；
菜单栏客户端必须同时接受带小数秒和不带小数秒的时间格式，不能把该解码失败误判为浏览器
打开失败。

同一 LAN DHCP 接管的恢复状态、source snapshots 和 operation records 保存在用户的
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
订阅完整 URL 存在 Keychain，
sources JSON 只保留脱敏 origin。

设备流量面板使用独立的受认证 `GET /api/v1/device-traffic`，不要在前端重复解释 raw
connections。后端按 DHCP lease IPv4 与 mihomo `metadata.sourceIP` 归属当前活跃会话，
设备合计排除无法匹配的网关本机/未知来源连接；主出口按累计字节最多的完整 chain 选择。
这是 `active_sessions` 快照，不是持久化历史。mihomo 不可用时仍返回 lease inventory，
并通过 `connection_error` 明确统计不可用。

HTTPS source 请求使用 mihomo/Clash Meta 兼容的 User-Agent，因为部分订阅服务会按
客户端标识选择响应格式。草稿只做结构校验；apply 只由 privileged helper 对最终候选
执行一次真实 `mihomo -t`。生成配置把 geodata 下载指向 MetaCubeX 官方仓库列出的
JSDelivr-CF 入口，下载结果保存在 applied profile 所在数据目录供后续校验与启动复用。

Source 的运行状态不能由 sources JSON 中的可变布尔值决定。desired profile digest 来自
当前 config 指向的 imported profile 内容，applied profile digest 只在 gateway `start`
成功时写入 runtime state；来源列表、Dashboard 和 SSE 都用两者比较 drift。运行中应用
source 是同步的两阶段事务：先写候选并做真实校验，再通过完整 `reload` 生成新的
`runtime/mihomo.yaml`；成功后才显示“运行版本”。停止时只显示“下次启动版本”。reload
失败必须恢复原 config；若旧 runtime state 已被清除，还要尝试用旧 config 重新启动，
不能留下“新 desired、旧 runtime、界面已同步”的假状态。运行中的 source apply 也要写
一条 reload operation 供诊断审计。

同一 LAN DHCP 接管 start 后还有 `client_validated` 阶段：要求 active lease、DHCPACK、客户端源 IP
DNS 与 mihomo TUN 日志，并保存用户对网关/DNS、无显式代理和 IPv6 绕过警告的确认。
Web GUI 允许用户显式进入 `client_validation_skipped`，然后继续 stop；这只是解除流程阻塞，
必须记录“没有客户端路径证据”，不能显示或对外宣称已经验收。紧急 stop 仍允许直接执行，
以免验收失败阻塞网络恢复。

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

网络配置通过 revisioned `GET/PUT /api/v1/config` 修改；只允许 topology、DHCP/DNS、
TUN 和 device-policy 初始化字段，运行中或 `prepared` 之后的 recovery 时拒绝。所有
production 写入经 helper 落到 root-owned config。`/events` 发送真实
config/gateway/drift/recovery 变化，诊断接口返回连接与脱敏后的短日志尾部。

设备页的主交互必须区分绿色“即时生效”和黄色“需重载”。前者只允许切换已应用的非
`device/` 全局组和 `device/<id>/<slot>`；后者编辑 desired 设备身份、路由模式、候选与规则。
设备路由模式是 `inherit_global`（跟随本机/全局规则）或 `dedicated`（公网流量优先设备
default selector，本地/私网保持直连）；缺失字段显示旧版兼容状态并要求显式迁移。
全局组说明不得暗示 macOS system proxy 或统一 fallback。登记面板复用 OpenSurge DHCP
租约自动填写 hostname、MAC 与 IPv4，默认创建 `<device-id>-policy` 私有 Profile；首次
编辑共享/Template Profile 时将解析后内容复制为无 Template 的设备私有 Profile。
Profiles/Templates/Rule Sets 作为高级复用机制默认折叠。

策略页承担完整节点健康中心：`GET /api/v1/proxy-health` 汇总 mihomo `/proxies`，
`POST /api/v1/proxy-health/tests` 只允许探测当前 snapshot 中的 leaf proxy，并使用固定
`generate_204` URL 和受限并发调用 mihomo delay API。策略页显示全部节点状态并允许
Selector 即时切换；设备页仅显示当前出口摘要，打开选择器后才展开候选。该探测是网关
Mac 上 mihomo 到检测地址的节点可达性，不是下游设备数据面证据。

连通性页使用后端固定 catalog，避免把任意 URL 探测变成 SSRF 接口。
`POST /api/v1/connectivity/tests` 从 Control Service 经 applied runtime mixed-port 发起
三轮请求，并在请求仍活跃时尽力关联 mihomo connection 的 rule、rule payload 和 chain。
它能证明 applied 全局规则路径，不证明设备 `SRC-IP-CIDR`、DHCP、DNS 或 TUN。页面把
Net.Coffee 明确标为浏览器本机外部检测，并把尚无真实客户端发起器的“设备端检测”显示
为不可用，不能把三种 scope 合并为一个模糊的“网络正常”。

安全重载入口是 desired 保存之后的 `POST /api/v1/gateway/reload`；运行中应用 source 也
进入同一 reload 生命周期。它复用 operation ID、
审计 store 与 helper 的窄权限 `reload` action；只接受 healthy running，先在临时 runtime
运行完整生成、静态检查、reservation 冲突和真实 `mihomo -t`，再完整 stop/start。预校验
失败不停止旧网关；same-LAN restart 失败回到 `router_dhcp_disabled_confirmed` 并触发跨页
恢复警告。不要把它描述为热替换或零中断。

Desired 网络配置默认把 `dns.upstream` 显示为 `127.0.0.1#1053`，形成
`dnsmasq -> mihomo fake-IP DNS`。旧配置中的空 upstream 在 dnsmasq 渲染时也迁移到这条
路径。`1.1.1.1` 只作为显式调试预设；TUN 的 `dns-hijack any:53` 仍可能捕获该查询，
因此 UI 不把它描述为可靠的直连或 TUN bypass。

用户可见产品文案把 `same_wifi_dhcp` 称为“同一 LAN DHCP 接管”，因为该协作式二层
拓扑可由 Wi-Fi 或以太网承载；`same_wifi_dhcp` 仅作为现有配置枚举和 runner 名称保留。

生产 pkg 使用固定 `/` install location 和不可 relocatable 的菜单栏 bundle，把 App 安装
到 `/Applications/OpenSurge Menu Bar.app`；否则 macOS Installer 可能把它 relocate 回
构建工作区的 `payload/Applications`。生产 pkg 把 applied config、mihomo/dnsmasq、runtime 和 helper 放在 root-owned 的
`/Library/Application Support/OpenSurge` / `PrivilegedHelperTools` 下；用户级 Control
Service 只通过 admin 组只读访问 applied 状态，通过 helper 执行固定 privileged 动作。
打包时 `OPENSURGE_VERSION` 必须同时写入 pkg receipt 与菜单栏 App 的 short version，
`OPENSURGE_BUILD_NUMBER` 写入 App build number，避免新安装包继续携带旧的 bundle 版本标识。

pkg 升级必须在覆盖 payload 前执行 recovery 门禁，并按 Control Service/菜单栏退出、
旧版 `omg stop`、root helper bootout 的顺序清理运行进程。recovery 非 `idle`/`complete`/
`complete_static` 或旧版网关停止失败时直接拒绝升级；`complete_static` 是明确保留 Mac
静态 IPv4 的终态，不应被误判为恢复未完成。postinstall 不得覆盖已有 `config.yaml`，导入源、
设备策略和 runtime 记录也必须跨升级保留。

开发期用 `make web-build`、`make menubar-build` 和 `make test`。这些检查不证明真实
DHCP、TUN 或 per-device 数据面；网络声明仍服从 validation-gates 页面。
