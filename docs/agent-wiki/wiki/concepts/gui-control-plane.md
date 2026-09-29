# GUI 控制面

Next 的完整 GUI 是 `web/` 中的 React 应用，由 `apps/desktop/` 中的 Wails +
系统 WebView 承载主窗口和菜单栏面板；浏览器入口继续可用。两者都只访问
`cmd/opensurge-control` 的 loopback API，Go 业务规则与 root Helper 保持独立。
当前宿主、认证和安装器契约见 [Desktop host](desktop-host.md)。下文涉及 Swift/AppKit
旧宿主的具体实现和故障记录仅供 v0.2.4 维护参考，不是 Next 的实现要求。

原生应用图标由 `apps/desktop/Resources/OpenSurgeAppIcon.png` 经
`scripts/build-desktop-app.sh` 等比生成各档 `.icns` 资源。1024 × 1024 源图已包含
透明留白：白色底板主体宽约 824 px、每侧留白约 100 px；构建时不要再次补同样的边距。
此比例用于传统 `.icns` 的视觉对齐，不是所有 macOS 图标格式的通用尺寸契约。
未来采用 Icon Composer 时应按其模板重新校准并验证系统实际渲染。菜单栏状态项使用
独立的 `OpenSurgeMenuBarIcon.png`，显示尺寸为 18 × 18 pt，不跟随应用图标的留白调整。

菜单栏 App 不提供 start/stop 或策略切换。它消费 `/api/v1/menubar` 显示网关、
drift 和恢复状态；运行时还只读 `/api/v1/device-traffic` 与 `/api/v1/local-routing`，
展示最近 60 秒流量、本机出口和活跃设备，并带设备筛选恢复同一个桌面主窗口。
显示与采样约定见 [Desktop host](desktop-host.md)。唯一独立的网关相关动作是
与网关状态无关的临时“合盖保持运行”开关；不要借此把菜单栏演变成第二套网关控制面。

界面语言同样保持单一设置入口：Web GUI 的侧栏提供“跟随系统 / 简体中文 / English”，
菜单栏面板不再增加第二个选择器。默认值是 `system`；用户没有选择过语言时，Web 按浏览器
第一语言、原生菜单按 macOS 首选语言的第一项决定显示语言，所有 `zh-*` 偏好
使用简体中文，其他语言回退到 English。Web 通过受认证的
`GET/PUT /api/v1/ui-preferences` 保存 `system | zh-Hans | en`，Control Service 将该偏好以
`0600` 写入用户数据目录的 `preferences.json`，并在 overview、menubar 与 state event 中
返回同一值。浏览器 localStorage 只用于避免首屏闪烁，不是第二份权威偏好；菜单栏通过现有
状态轮询最终同步 Web 的选择，在取得第一份状态前仍按本机语言显示。网络模式的三张拓扑图
使用 React 内联 SVG，共享主题色并通过同一文案目录翻译 `<title>`、`<desc>` 与图内标签，
不要恢复成明暗主题各一套、无法随语言变化的静态 SVG。
新增或合并的 Web GUI 页面必须把所有面向用户的中文传入统一 `t()`
目录，不能只翻译导航和旧页面。`make web-test` 中的 `check-i18n.mjs`
是覆盖门槛；新功能的英文组件测试还应展开主要对话框/预览，断言可见
内容不再包含 CJK 字符，避免仅首屏看起来已翻译。

睡眠开关默认关闭、不写 preference，只由 Control Service 内存中的 lease 表示本次运行
意图。菜单栏与 Web GUI 都调用 `PUT /api/v1/sleep-prevention`；Control Service 保持到 root
Helper 的长连接，连接 EOF、完整退出或服务崩溃都会释放。普通 `caffeinate` 的 idle sleep
assertion 不能覆盖 lid close，因此 Helper 使用系统级 `pmset -a disablesleep`。启用前若
`SleepDisabled` 已由外部设置为 1，Helper 拒绝接管；只有创建了 root-owned marker 后才
会在释放时写回 0。marker 位于持久的系统 runtime 目录，必须先于 `pmset` 写入，确保系统
重启、Helper 重启、pkg 升级和卸载能识别并恢复 OpenSurge 遗留的临时接管。UI 必须提示
耗电、发热和不要放入不通风包内。

两端不能缓存独立的开关意图。PUT 成功后应立即采用响应中的 lease 状态；Web SSE 的
`state` 变化签名包含 `sleep_prevention`，因此菜单栏切换会促使已打开的 Web GUI 刷新。
菜单栏打开时的快速轮询和 Web overview 定时读取负责最终收敛，并且较早发出的状态请求
不得覆盖较新的 PUT 结果。

版本发现属于原生 App 生命周期而不是网关控制面。菜单栏 App 打开时至多每 24 小时查询
一次本仓库 GitHub `releases/latest`，也提供手动检查；只比较稳定版语义版本并校验返回的
下载页仍位于 `YTwsy/OpenSurge-for-Mac`。发现新版本后只打开对应 Release 页面，不下载
PKG、不请求管理员权限，也不触发 gateway、Control Service 或 Helper 生命周期动作。
打包时额外写入完整 `OpenSurgeReleaseTag`；比较遵循
`0.1.24-rc.1 < 0.1.24`，因此 RC 不会降级到旧 stable，同版本 stable 发布后仍会提示。
旧包没有该 key 时才回退到 `CFBundleShortVersionString`。

<details>
<summary>历史 Swift 宿主的 AppKit 面板经验（仅适用于 v0.2.4）</summary>

旧菜单栏 App 使用纯 AppKit `NSApplication` 生命周期，不声明占位的 SwiftUI `Settings`
Scene；否则这个由系统管理、可恢复的空窗口可能在部分 macOS 环境中被显示。状态面板使用
AppKit `NSStatusItem` + `NSPopover` 承载现有 SwiftUI `MenuContentView`。状态栏图标点击
与用户从 Finder/Launchpad 再次打开
`/Applications/OpenSurge.app` 进入同一个 presenter；后者只确保面板展开，不触发网关
或 Control Service 生命周期动作。每次菜单栏 App 进程启动完成都会主动展开面板，包括
通过“登录时显示”启动；Finder/Launchpad 的 reopen 事件同样确保面板展开。
首次启动时，`NSStatusItem` 的按钮可能尚未附着到可见窗口；此时 AppKit 会忽略
`NSPopover.show`。菜单栏 presenter 必须在状态栏按钮拥有 window 且可见后展开，但不能把
`NSApplication.isActive` 或 popover window 的 `isKeyWindow` 当作展示前置条件：macOS 14
及以上的 cooperative `NSApplication.activate()` 可能拒绝 LSUIElement App 的请求，激活与
`makeKey()` 都只能是展示后的 best-effort 增强。

App 未 active 时，popover 临时使用 `.applicationDefined`，由状态栏按钮、Escape 与全局鼠标
监听负责关闭；App 获得 active 后再切为 `.transient` 并尝试令真实 window 成为 key。
SwiftUI 面板显式使用 active control appearance，状态栏按钮以持久 `state` 表示面板已展示；
状态轮询仅在 indicator 改变时重绘图标，不能抹掉展示状态。流程由
`applicationDidBecomeActive`、popover delegate 与 common run loop 上短时、有界的退避重试
推进，不接入高频 `applicationDidUpdate`。`NSPopover.isShown` 只表示调用过 `show`，还必须
给动画留出 window 创建宽限期并确认真实 window；超时后执行一次非阻塞兜底展示并清除
pending，不能留下永久卡住状态。macOS 13 仅保留兼容的旧 activation fallback。

不要再尝试"把面板 focus 工作推迟到展开动画结束之后"这一类改法。`codex/release-v0.1.24`
上曾连续提交四版实现：无条件 activation fallback、把展开推迟到 reopen 激活、用
`popoverDidShow` 作为动画完成信号、以及用固定 settle 时间窗跳过
`makeKeyAndOrderFront`。它们都没有修复实际报告的展开异常，反而引入了新的问题，已被
整体回退到 `0932641`。若要重开这个方向，先给出可复现的 WindowServer 级证据和真机
验收，不要只依赖单元测试与 `scripts/check-menubar.sh`。

</details>

Web GUI 总览页的“启动网关”与“停止网关”只导航到 `network` 页面，不得直接调用
gateway start/stop API。真实生命周期动作留在网络页，使 topology、plan blocker、DHCP
接管与恢复状态在用户确认前保持可见。“启动网关”只切换页面，不改变当前滚动位置；
“停止网关”切换页面后滚动到页面底部，完整露出恢复状态机的当前操作按钮。

策略页不再依赖网关已经启动。运行中读取真实运行 core；停止时由 root Helper 通过长连接
lease 维持一个仅开放随机、带 secret 的 loopback Controller 的 prepared mihomo。页面从同一
workspace API 获取最终策略组、选择与健康状态，因此 Provider、全局附加节点和 Tailscale
Exit Node 都以最终合成结果出现，选择写入与下一次网关共用的 mihomo cache。prepared core
不能开启 mixed/SOCKS/HTTP 业务端口、DNS listener、TUN、DHCP、pf、forwarding 或 IPv6
packet ingress；Control Service 退出/断连、Helper 重启以及真实 gateway start/stop 都必须
回收或交接它，且不能根据未验证 PID 杀进程。

没有导入 mihomo YAML 也是策略页的一等状态。启用的全局附加配置以 OpenSurge 的 managed
最小 profile 为基底，可以独立添加节点、`select` 组与规则。停止态策略页的 read/select/test
只生成准备态产物并使用原有选择/测速缓存，不修改 desired 或基础恢复记录。用户可以不打开策略页，
直接从 Web GUI 启动；Control Service 会把服务器端读取的同一来源/附加配置快照交给 Helper，
Helper 在一个跨进程 lifecycle lock 内合成候选，由 Manager 解析最终网络参数、渲染并执行
一次真实 `mihomo -t`，成功后才提交 desired 并接管网络；失败不能退回旧配置启动。策略页因此是可选的预览、选择与测速入口，
不是启动前置步骤。来源、附加配置或 Tailscale 任一为空都必须返回正常的空/部分策略快照，
而不是崩溃。运行中仍只展示 applied core，不能因为轮询策略页而把尚未显式应用的草稿热重载
进当前网关。`sudo omg start` 只启动已经持久化的 root-owned desired 配置，不读取用户 Control
Store 中的 Web 草稿。仅附加配置的运行中草稿通过下一次 App 启动采用。
候选错误必须显示具体原因并清除旧成功快照；Provider 首次加载尚无节点属于正常空状态。

workspace 请求同时携带 Control Service 捕获的 gateway running/stopped 状态。Helper 在跨进程
lifecycle lock 内重新读取 state；若 CLI 或另一客户端已经完成 start/stop，本次 read/select/test
或 start 必须返回刷新/重试，而不能跨状态复用快照。HTTP/file Provider 沿用既有相对、绝对
和缺省路径解释，不做通用缓存重命名。新合成产物、选择缓存与 Tailscale 身份共用固定的
受信任工作目录；composition base metadata 位于 runtime 控制目录，并只随显式提交更新。

网络页对 `same_wifi_dhcp` 保留带恢复证据的完整状态机；`same_lan` 与 `isolated_lan`
使用独立“网关运行控制”卡片直接调用 start/stop operation。未保存配置必须阻止启动，
但不能阻止运行中网关的安全停止；`degraded` 仍属于运行中状态。两种直接启停路径都要
显示 topology 对应的影响范围并要求显式确认。

菜单栏 indicator 先判断需要用户处理的 recovery，再判断 gateway 是否明确 `stopped`；
只有正在运行或 degraded 的 gateway 才把 drift/doctor failure 表示为“运行异常”。停止状态
下的 runtime doctor failure 或待应用配置不能覆盖“OpenSurge 网关已停止”。

Doctor 包含真实 `mihomo -t`，单次配置验证最长可到 90 秒，因此不得从
`/api/v1/overview`、`/api/v1/menubar`、SSE 或其他轮询热路径同步执行。Web 诊断页通过
`POST /api/v1/doctor` 显式启动 Control Service 内的 single-flight 后台检查，再用
`GET /api/v1/doctor` 读取运行状态和缓存结果；重复请求只能观察同一份进行中的任务。
缓存以主配置、设备策略与 imported profile 摘要共同标识，配置变化后只能显示为旧结果，
不能继续影响当前菜单栏健康状态。这个只读 Doctor 缓存不参与 start/reload 放行；两者仍须
执行各自的真实预检与 TUN readiness，不能用历史 Doctor 成功结果替代。

进程刚启动且尚未取得第一份状态时使用独立的 connecting 状态和 OpenSurge 品牌图标；真实
请求失败后进入 unreachable。Next 使用状态符号、tooltip 和面板文案区分各状态，不沿用
旧 Swift 宿主的图标透明度约定；Control Service 尚未准备好不表示网关已经停止。

“只退出桌面 App”关闭主窗口和菜单栏，不会停止用户级 Control Service，也不会停止正在
运行的 DHCP/DNS、mihomo、PF 或 forwarding。该按钮必须先显示状态感知的二次确认；
网关运行时应明确列出仍会继续的服务，状态不可达时应提示先检查，而不是暗示后台已退出。

“退出 OpenSurge”是独立的近期退出层级：只有网关数据面已经停止且没有待处理网络恢复时
可用，并同时结束桌面 App 与用户级 Control Service。它不 bootout 系统 launchd 托管的
root Helper；Helper 保持空闲加载，因此重新打开 App 或重启电脑都不要求再次授权。正式安装的
App 重新打开时必须能从已安装的 LaunchAgent plist bootstrap 此前被 bootout 的 Control
Service。只有卸载、重新安装或修改系统级 Helper 才进入需要管理员授权的边界。

宿主 `net.inet.ip.forwarding` 是全局状态；用户可能在 OpenSurge 启动前已经启用它。
因此 raw `forwarding == enabled` 不能单独算作 OpenSurge 服务仍活跃，也不能阻止完整退出
或卸载。gateway manager 仍必须记录并恢复启动前 forwarding 值。

菜单栏与 Web GUI 总览用“IPv4 接管”和“IPv6 接管”展示按地址族归一化后的运行状态，
不能把 raw forwarding 直接改名成 IPv4 接管。IPv4 只有在当前 boot 的 gateway runtime
active、PF anchor loaded、forwarding enabled 且整体 gateway running 时才显示正在接管；
停止态即使宿主原本已经启用 forwarding 也显示已停止。IPv6 接管来自用户态 packet path，
区分正在接管、自动模式等待上游、已关闭、已停止、异常与重启后待清理。底层
`forwarding`、`ipv6_packet`、`native_ipv6_available` 和 `ipv6_reason` 继续保留用于诊断。

菜单栏提供独立“卸载 OpenSurge”入口。卸载只以 `gateway == stopped` 为门禁，不受
recovery 阶段影响；确认窗口允许保留配置/订阅/策略数据或彻底删除全部数据。管理员授权
后只调用 pkg 安装到固定系统目录、root 拥有的卸载脚本；脚本必须自行再次确认 gateway
stopped，再移除用户 Control Service、系统 Helper、App 和 receipt。升级继续使用 pkg
preinstall 的严格 recovery 与 stop 顺序，不能与产品卸载门禁混为一谈。

桌面退出使用原生确认，完整退出在确认后再次检查最新状态。退出流程必须阻止并发重新
连接；Control Service 的 wake 与 bootout 串行化，确保退出路径的最终生命周期动作是 bootout。
旧 Swift 宿主使用同步 `NSAlert.runModal()`，是为了避开已观察到的 SwiftUI alert 回调丢失；
这段历史不要求 Wails 沿用 SwiftUI 或同一个确认实现。

旧 Swift 菜单栏通过 `NSWorkspace.shared.open` 打开浏览器，失败后回退 `/usr/bin/open`。
Next 的菜单栏直接恢复主窗口；native relay 负责会话交换，不将 bearer token 或 cookie
交给 JavaScript。任何宿主都不能把一次性 bootstrap URL 写入错误信息或长期日志。

Control API 的 bootstrap `expires_at` 来自 Go `time.Time`，可能包含 RFC3339 小数秒；
菜单栏客户端必须同时接受带小数秒和不带小数秒的时间格式，不能把该解码失败误判为浏览器
打开失败。

Web GUI 从一次性 bootstrap 链接换取的 HttpOnly 会话使用 12 小时闲置期限；每次有效的
浏览器会话请求都会滑动续期，因此持续打开的控制面不能在固定时间点突然失效。会话仍只
保存在 Control Service 内存中，不跨服务重启持久化。浏览器收到 401 后必须停止 API 轮询
与 SSE，并明确引导用户点击 macOS 菜单栏中的 OpenSurge 图标，再选择“打开 OpenSurge 面板”；
不能继续提供必然再次返回 401 的普通“重试”按钮。

菜单栏的 Control API bearer token 只从用户应用支持目录内权限为 `0600` 的
`control-token` 读取，不复制到 Keychain，也不回退到可能过期的旧 Keychain 副本。文件
缺失与 endpoint 尚未生成都表示用户级 Control Service 尚未准备好：先轻量 kickstart 并
重试，仍失败才显示友好错误和“重新连接”。Next 的启动与显式重新连接都不使用
`kickstart -k`，只唤醒已有服务，不强制重启 Control Service 或操作网关数据面。
Preview 默认不管理已安装服务；旧 Swift 的重启实现只供历史版本维护。

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

初次启动、停止、重载、Mihomo 恢复、路由器 DHCP 关闭/恢复 OFFER 检查，以及设备策略/
来源/Tailscale 配置应用，复用全局 operation 进度卡。客户端提交即显示等待状态，后续 `phase`、`phase_started_at`、
`notices` 来自 Go 生命周期实际边界；只显示阶段与耗时，不模拟百分比。进度卡在页面
切换后继续显示，刷新时只恢复未完成操作，不重新弹出旧完成记录。当前没有未收起的进行中
操作时，只展示最新操作的结果；该结果被关闭或自动消失后，不回退展示旧成功或失败。
轮询更新不能改变同一创建时间下的首次登记顺序，旧记录仍保留供诊断查看。
DHCP 检查复用同步请求的关联 ID，探测期间报告 `probing_dhcp`；只有探测结果符合要求且
恢复状态保存成功后，才显示完成。未收到 OFFER 只表示本次探测结果，不扩大为路由器状态的
绝对保证。网络页的 DHCP 接管 client acceptance 不会被“启动完成”替代。

Helper 请求可选 `watch_progress`：新 Helper 先发送带 `progress` 的 JSON 帧，最后仍
返回原有结果；旧客户端不请求该字段时只收到最终帧，新客户端也兼容旧 Helper 的单帧
响应。进度写入超时后停止观察，不能因为 UI 断开而阻塞网络清理。同步配置 API 仍保持
原请求/响应契约，可用 `X-OpenSurge-Operation-ID` 关联进行中的记录；该字段不是重放
授权，重复 ID 拒绝，ID 仅允许 1–128 个字母、数字、连字符或下划线。

同一操作共享状态轮询；读取失败可重试 GET，但不得自动重试原 POST/PUT。浏览器断连
或等待超时应显示“结果尚未确认”，保留原 ID 供重新查询和诊断，不将其当作网关已停止、
已失败或应该重新启动。认证失效继续遵守停止轮询与 SSE 的既有边界。

订阅完整 URL 存在用户应用支持目录的独立 `credentials/sources.json`，目录权限为 `0700`、
文件权限为 `0600`；公开 sources JSON 只保留脱敏 origin，API、日志和诊断不得返回刷新
凭据。升级只尝试一次从旧 `com.opensurge.sources` Keychain 项迁移，并无论成功或失败都
写入标记，以免后续启动继续访问 Keychain；旧项不自动删除。迁移失败不能阻止 Control
Service 启动，已有来源快照继续可用，用户可重新导入 URL 恢复刷新能力。

总览使用受认证的 `GET /api/v1/device-traffic`，连接页使用 `GET /api/v1/connections`。
两者复用同一份后端归属、计数器差值和一秒内共享的采样；配置、运行状态、租约或登记
变化会使缓存失效。不要在前端重复解释 raw connections 的设备身份。后端用 DHCP lease、
applied 设备、desired 登记和当前观察到的网关 LAN 源 IPv4 建立清单。只有已进入 applied
bundle 的 compiled devices 可以作为运行中的登记身份；未应用、暂停的 IP-only 登记和
网段外设备保留在清单中，但不能作为归属证据。IPv4 仍按源地址归属，冲突身份不强行合并。
本机身份由
`internal/controlapi/gateway_local.go` 统一判断：系统 TUN 快照额外读取一次 mihomo
`/configs` 的实际 `inet4-address`/`inet6-address`，同时要求 `type=Tun`、
`inboundName=DEFAULT-TUN` 和精确本机源地址。只取接口地址，不取整个 CIDR；下游也
经过 `DEFAULT-TUN`，并可能具有相同 `inboundIP`，所以入口字段不能单独证明本机来源。
回环/网关 Mac 源地址仍作为本机身份，但 `opensurge-ipv6` listener 和
`inboundUser=device:…` 优先排除。累计流量、速率和本机关闭连接操作复用这个判断。

不能用 `process/processPath` 或共享源 IP 推断本机身份。mihomo 默认 `strict` 只在
规则需要时查询进程；订阅有无 `PROCESS-NAME`、规则顺序、进程查询失败都不能改变
流量归属。不能通过强制 `find-process-mode: always` 代替身份修复。IPv6 地址必须来自
实际运行状态，不能按 desired `auto` 或旧快照同时猜测 fake-AAAA 与显式 TUN 两种身份。
读取 TUN 身份失败时，流量 API 返回 `connection_error` 并保留可确认的清单；本机关闭
连接接口返回 `local_identity_unavailable`，不部分执行。纯显式代理快照不额外读 `/configs`。

本机连接聚合到独立 `gateway_local`，不能把 Mac 放进 `devices`、下游设备数量或策略身份模型。GUI 在“活跃
设备”中固定把“本机 Mac”显示为第一行，并根据实际 connection type 显示 TUN、显式代理
或两者；网关停止时显示网关未运行。

没有 DHCP/静态身份但能确认网关 LAN 源 IPv4 的行使用 `observed_traffic`，其连接数进入
`unidentified_device_connections`，GUI 称为“待识别设备连接”。地址缺失、网段外且没有
本机证据等剩余连接进入 `unclassified_connections`，可在连接页的“无法归属”中查看。
`unmatched_connections` 是旧客户端兼容字段，当前 GUI 不再用它解释来源身份。
`identity_source` 必须区分 `gateway_local`、`dhcp_lease`、`registered_static` 与
`observed_traffic`；无法归属的独立汇总行使用 `unclassified`。主出口按累计字节最多的完整 chain 选择。

下游 IPv6 归属必须同时满足 `type=Tun`、`inboundName=opensurge-ipv6`、合法 IPv6
源地址和 `inboundUser=device:<id>`，且 ID 对应当前 LAN 中带 MAC 的有效 applied 设备。
同一设备的 IPv4、多条 IPv6 隐私地址聚合到 `device:<id>`；未知 ID、其他 listener、
缺少身份或 MAC 冲突保留为无法归属，不能按 IPv6 前缀或进程猜测。本机精确 TUN 身份
仍优先排除下游身份。`connections[].owner_key`、`source_family` 和速率由后端统一给出；
`gateway_totals = gateway_local + totals + unclassified`，计数、会话字节和速率均可核对。
这里的网关总量也只表示当前活跃连接，不等同于网卡累计字节或 mihomo 历史总量。

连接页是独立一级导航，设备页继续负责配置，诊断页负责操作/Provider/日志。总览与设备页
可按 owner 深链接到连接页。筛选、排序、选择设备和滚动位置在切页返回时保留；每页最多
显示 50 条连接。暂停会取消进行中的请求并固定画面，恢复后重新采样；页面隐藏或卸载时
停止有效更新。连接离开快照后仅保留选中项的最后详情，不把它扩展成历史连接日志。
“有租约”“已应用登记”“观察到流量”分别展示，没有活跃连接不表示设备离线。
主路由旁路的 IPv4 显示不在统计范围内。网络错误保留上次快照并标注过时；核心不可用
仍返回清单并隐藏统计；租约/登记读取错误单独以 `inventory_error` 标明清单可能不完整。

这是 `active_sessions` 快照，不是持久化历史。mihomo 不可用时仍返回本机、lease 与
applied 静态设备 inventory，并通过 `connection_error` 明确统计不可用。实时 bytes/s
由 Control Service 在内存中按 connection ID 比较相邻采样得到；首次、新连接、长采样
间隔和错误恢复先建立基线。GUI 每 2 秒读取一次，只保存最近 60 秒趋势；总览和点击本机/
设备后展开的趋势卡复用同一图表语义，不能把这段内存数据描述为今日/月度历史。图表使用
平滑曲线；速率数字必须在新采样到达时立即更新，只有曲线在相邻前端采样之间使用约
700 ms 的缓出插值；系统请求减少动态效果时曲线也必须直接采用新值。网关总趋势保持
紧凑并只留低对比参考线。宽屏设备详情不能参与设备列表行高
计算，否则展开/收起会改变文档高度并造成页面底部滚动跳动；窄屏纵向展开则需要显式
高度过渡。

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

来源页必须分别显示导入、刷新、完整校验/应用的进行中状态，并在动作完成后保留成功或
错误反馈。总览 GATEWAY 卡片直接使用 overview 的当前配置 `topology`，并在同一张卡中
展示接口、LAN IPv4、接管模式和 desired/applied 状态；不要从 `recovery.topology` 猜当前
模式，也不要再增加一条重复的网络上下文条。

来源列表只公开经过管理目录校验的 `snapshot_display_path`，继续清空原始
`snapshot_path` 与订阅取回地址。复制完整路径、Finder 定位和导出副本分别通过受认证的
来源文件接口执行；动作前必须同时校验 source ID、digest、规范化路径、私有权限和文件
内容摘要，不能把任意持久化路径交给 Finder。管理快照只读，导出副本写入 Control Service
用户数据目录下的 `exports/`，目录权限 `0700`、文件权限 `0600`，命名包含来源名、摘要短值
和时间戳且不得覆盖旧文件。导出成功后用用户会话中的 Finder 选中新文件；如果 Finder
调用失败，已经写出的副本必须保留，并在错误反馈中给出其显示路径。

局域网 DHCP 接管 start 后还有 `client_validated` 阶段：要求 active lease、DHCPACK、客户端源 IP
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
TUN、本机系统代理协同和 device-policy 初始化字段，运行中或 `prepared` 之后的 recovery 时拒绝。所有
production 写入经 helper 落到 root-owned config。`/events` 发送真实
config/gateway/drift/recovery 变化，诊断接口返回连接与脱敏后的短日志尾部。
上下游接口字段通过只读 `GET /api/v1/network/interfaces` 提供 macOS 网络服务候选，
但仍保留可输入形式以支持没有列入网络服务顺序的 bridge、VLAN 或临时接口。
安装器初始网络字段尚未保存为用户配置时，选择 `same_lan` 或 `same_wifi_dhcp` 会通过只读
`GET /api/v1/network/defaults` 读取当前 IPv4 默认路由对应的网络服务，把同一接口、当前
IPv4、子网前缀与 `dns.listen` 写入前端草稿；`same_wifi_dhcp` 还会在该网段内生成避开
Mac、路由器和受保护地址的建议池。同一条路径还挂在网络页的「根据当前网络重新填入」
按钮上，供 Mac 换网络后手工对齐。建议不自动保存、不执行 `networksetup`，
已有配置也不得被静默覆盖。
`isolated_lan` 不使用这条建议路径，继续由操作者手工配置独立下游接口和子网。
`same_lan` 不运行 DHCP 服务，因此地址池与租期整组必须禁用并明确标记为运行时不使用；
保留字段值只用于日后切换 topology，不能暗示当前模式会应用它们。
配置填写提示应作为表单内的低强调步骤说明，保存区与最后一组字段保持明确间距，并显示
当前已保存或存在未保存修改，避免按钮紧贴字段卡片。设备页的未保存修改与已保存待重载
状态使用同一套底部浮动操作条；保存后直接从“保存设备配置”切换为“应用并重载网关”，
不把待重载提示移回页面顶部。

设备页先显示独立的 Mac 本机模式卡片；它只调用 `GET/POST /api/v1/local-routing`，
在规则 / 全局 / 直连之间协调 `open-surge/mac-*` 隐藏 selector。卡片必须说明只影响
TUN/本机显式代理的新连接，且自身不修改 macOS system proxy 或下游设备。系统代理只由
Desired 网络配置中默认关闭、仅 TUN 可用的独立兼容开关管理。

下游设备交互继续区分绿色“即时生效”和黄色“需重载”。前者只允许切换已应用的
`device/<id>/<slot>`；后者编辑 desired 设备身份、路由模式、候选与规则。设备路由模式
是 `inherit_global`（跟随 imported/managed 网关规则，不跟随 Mac 本机开关）或
`dedicated`（公网流量优先设备 default selector，本地/私网保持直连）；缺失字段显示
旧版兼容状态并要求显式迁移。DHCP 模式的登记面板复用
OpenSurge lease 自动填写 hostname、MAC 与 IPv4；`same_lan` 则列出 mihomo 当前观察到且
与 gateway 同网段的源 IPv4，并用 macOS ARP 邻居表尽力补 MAC。只有当前经过 Mac 的
设备会出现，ARP/流量观察不得显示为 DHCP 验证。登记默认创建 `<device-id>-policy` 私有 Profile；首次
编辑共享/旧式 Template Profile 时将解析后内容复制为设备私有 Profile。
主界面不把 Profile 作为复用对象：默认展开的“规则库”只显示规则集、无出口分流模版和
设备分流。设备卡的“编辑设备分流”直接打开对应设备；旧的独立设备规则卡片和
“高级 / 复用”卡片不再渲染。

运行态实际切换 Mac 本机模式/固定出口或已应用设备的 selector 后，Web GUI 在全局
右下角保留作用域明确的连接刷新提示；切页不会丢失，同一对象连续切换只保留最新选择。
它默认不打断连接，必须由用户显式调用
`POST /api/v1/local-routing/connections/refresh` 或
`POST /api/v1/devices/<id>/connections/refresh`。刷新按最新快照关闭该作用域内当前由
OpenSurge 管理的全部匹配连接，并不只筛选仍使用旧出口的连接，也不保证客户端自动重连。
因此文案必须提示下载、通话等可能中断。停止态预选和重复选择不产生刷新邀请；卡片内原有的手动刷新入口继续保留。

策略与节点健康页切换普通共享 selector 后，同样提供可跨页面保留的刷新提示，使用
`POST /api/v1/policies/<group>/connections/refresh`。后端校验该组存在于运行中核心的
可见策略组中，再读取最新 connections，以 `chains` 中完整组名的精确匹配选择连接。
该范围包含实际经过此组的 Mac、跟随网关规则的设备，以及通过嵌套策略引用该组的设备；
它与设备登记模式、IPv4/IPv6 或当前叶子节点无关。使用同一节点但未经过此组的连接不得
关闭，也不能通过逐台刷新设备来扩大到其无关连接。刷新仍包含已走新出口的匹配连接，
并由客户端决定何时重连。组名在 URL 中编码，内部 `open-surge/mac-*` 组继续走原有本机
接口。切换本机专用全局出口不会改变下游规则，也不扩大其本机刷新范围。

设备卡自身提供整设备管理：「编辑身份与路由」复用登记面板并预填现有身份，「删除设备」
同时清理该设备的私有 Profile。两者都只改本地草稿，仍走同一次“保存设备配置”。设备
policy 是整文档提交，因此这些入口是操作者修正身份的唯一途径；`out_of_lan_devices`
标记的设备只能在这里改地址或删除，界面必须把这条出路说清楚。

策略页承担完整节点健康中心：`GET /api/v1/proxy-health` 汇总 mihomo `/proxies`，
`POST /api/v1/proxy-health/tests` 只允许探测当前 snapshot 中的 leaf proxy，并使用固定
`generate_204` URL 和受限并发调用 mihomo delay API。策略页显示全部节点状态并允许
Selector 即时切换；设备页仅显示当前出口摘要，打开选择器后才展开候选。该探测是网关
Mac 上 mihomo 到检测地址的节点可达性，不是下游设备数据面证据。

运行中与未启动配置的策略页统一使用 `POST /api/v1/policy-workspace`。节点检测
通过 `Accept: text/event-stream` 请求逐个结果：每个完成的节点发送 `result`，整批
完成发送包含权威 snapshot 的 `complete`，中途失败发送 `error`。未请求流式返回的
调用者继续收到原有 JSON。Helper 通过可选 `watch_policy_results` 转发节点结果，
保持同一个 workspace lease、生命周期锁、服务端配置来源和 6 路并发。前端只更新
对应节点的健康信息并结束其加载状态，不改变列表顺序；断流保留已完成结果，不重放
检测请求。页面卸载会中断结果订阅和剩余检测。

连通性页使用后端固定 catalog，避免把任意 URL 探测变成 SSRF 接口。
`POST /api/v1/connectivity/tests` 从 Control Service 经 applied runtime mixed-port 发起
三轮请求，并在请求仍活跃时尽力关联 mihomo connection 的 rule、rule payload 和 chain。
loopback 来源会进入当前 Mac 本机模式，因此 scope 是 `local_mac_runtime`；它证明
applied 配置 + 本机运行路径，不证明下游网关规则、设备 `SRC-IP-CIDR`、DHCP、DNS 或
TUN。页面把
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

上游 DNS、`transparent.mode` 与 `mihomo.store_fake_ip` 位于默认折叠的“高级 Mihomo /
DNS 设置”。`store_fake_ip` 对新配置和缺少该字段的旧配置默认开启，并生成
`profile.store-fake-ip: true`；旧 schema-v1 客户端省略该字段时，Control API 必须保留
现值，不能静默关闭。关闭开关只影响后续运行配置，不会清除已经保存的映射。缓存清理应
作为独立显式动作，不能与 `fake-ip-filter` 或持久化开关混为一谈。

Desired 网络配置同时提供 `local_system_proxy.enabled`。文案必须说明 SafeDNS、DNS
Proxy/内容过滤等已知用途、只覆盖遵循系统代理的 Mac 应用、不替代 TUN、不影响下游设备，
以及已有 HTTP/HTTPS proxy、PAC 或自动发现时启动会 fail closed。关闭 TUN 时前端应同时
关闭并禁用该开关，后端验证仍作为最终边界。

用户可见产品文案把 `same_wifi_dhcp` 称为“局域网 DHCP 接管”，因为该协作式二层
拓扑可由 Wi-Fi 或以太网承载；`same_wifi_dhcp` 仅作为现有配置枚举和 runner 名称保留。

生产 pkg 使用固定 `/` install location 和不可 relocatable 的桌面 bundle，把 App 安装
到 `/Applications/OpenSurge.app`；否则 macOS Installer 可能把它 relocate 回构建工作区的
`payload/Applications`。升级的 postinstall 在新 payload 落盘后清理旧的
`/Applications/OpenSurge Menu Bar.app`，避免 Launchpad 出现重复入口。主程序在 Next 改为
`OpenSurgeDesktop`，bundle identifier 与 launchd label 保持既有技术命名。
生产 pkg 把 applied config、mihomo/dnsmasq、runtime 和 helper 放在 root-owned 的
`/Library/Application Support/OpenSurge` / `PrivilegedHelperTools` 下；用户级 Control
Service 只通过 admin 组只读访问 applied 状态，通过 helper 执行固定 privileged 动作。
打包时 `OPENSURGE_VERSION` 必须同时写入 pkg receipt 与桌面 App 的 short version，
`OPENSURGE_BUILD_NUMBER` 写入 App build number，`OPENSURGE_RELEASE_TAG` 写入完整 stable/RC
tag；tag 的基础版本必须与 pkg 版本一致，避免新安装包继续携带旧的 bundle 版本标识，或让
RC 丢失其预发布身份。
没有 Apple Developer 身份的 GitHub tag workflow 会产生 Apple Silicon 与 Intel 两个
架构专用的 unsigned 正式 Release 安装包：文件名分别带 `arm64-unsigned.pkg` 与
`x86_64-unsigned.pkg`，发布同时提供合并的 SHA-256 清单和每个 pkg 的 GitHub artifact
attestation。正式 Release 只表示 GitHub 发布通道稳定，不得把 GitHub attestation 当成
Developer ID 签名或 notarization，也不得指导用户全局关闭 Gatekeeper 或递归移除
quarantine；安装仍使用系统设置针对单个包的“仍要打开”。

pkg 升级必须在覆盖 payload 前执行 recovery 门禁。进程清理必须先终止菜单栏 App，阻断
其“Control Service 不可用时自动 bootstrap”的恢复路径，再循环 bootout 精确的用户级
Control Service 并重新扫描已安装可执行文件；等待期间新出现的受信 PID 也必须再次清理，
避免一次迟到的 `launchctl bootstrap` 令首次安装失败。完成 GUI/Control 清理后，才执行
新安装包脚本目录内携带的当前版本 recovery CLI 和 root helper bootout；不得依赖即将被
替换版本的 `omg stop` 处理跨重启 runtime。recovery 非 `idle`/`complete`/
`complete_static` 或网关安全清理失败时直接拒绝升级；`complete_static` 是明确保留 Mac
静态 IPv4 的终态，不应被误判为恢复未完成。postinstall 不得覆盖已有 `config.yaml`，导入源、
设备策略和 runtime 记录也必须跨升级保留。

开发期用 `make web-build`、`make desktop-production-build` 和 `make test`。这些检查不证明真实
DHCP、TUN 或 per-device 数据面；网络声明仍服从 validation-gates 页面。
