# GUI 控制面契约

本页维护共享 React 界面的状态展示、页面职责与交互边界。HTTP 认证、存储和事务由 [Control API](../control-api.md) 维护，原生窗口与菜单栏由 [桌面宿主](desktop-host.md) 维护。

## 展示状态与诊断

总览 GATEWAY 卡片直接使用 overview 的当前配置 `topology`，同时展示接口、LAN IPv4、
接管模式与 desired/applied 状态；不能从 `recovery.topology` 猜当前模式，也不增加重复的
网络上下文条。来源页的导入、刷新、完整校验/应用须在触发按钮上显示进行中状态，并在
完成后保留成功或错误反馈，不能只用全页 disabled 表示受理。

overview、menubar 和 state event 通过同一 `presentation` 契约提供展示状态，主窗口
侧栏、总览卡和原生菜单栏消费该结果。底层 `status.gateway` 保持进程/运行时观测语义，
不能用于推断启动或重载已完成。Control Service 将当前实例实际拥有的生命周期操作与
观测结果组合为启动、重载、停止、代理引擎恢复、回滚或稳定状态；跨进程生命周期锁
覆盖外部 CLI 的受控变更窗口。遗留 operation 文件不能证明动作仍在执行；当前动作超过
原有三分钟上限时展示结果未确认。采样跨越操作开始/完成边界时有界重读，无法稳定确认
则显示未知，不把中途缺失的服务报为新故障。

策略 workspace 的读取、选择与节点检测也持有同一把生命周期锁，但不代表网关正在
启停或应用配置。Control Service 按请求生命周期记录这类本地活动：保持 `busy` 的
并发保护，同时继续展示实际运行、停止或故障状态，不生成全局操作进度，也不将其
误判为外部 CLI 操作。JSON 与流式检测在完成、失败或取消后都释放活动记录；长期
prepared-core lease 本身不表示网关忙碌。

`drift` 只表示待应用配置，显式 Doctor 的失败只表示诊断待检查项，两者不把正常运行
改为 degraded。实际进程缺失、TUN 明确关闭、PF 未加载、IPv4 forwarding 关闭或
下游 IPv6 数据面故障才生成相应故障原因；读取不可用展示 unknown。网络恢复要求独立
保留。展示状态不替换原始服务证据；完整退出与卸载同时拒绝 busy/unknown 窗口。
主窗口丢弃较旧请求的迟到响应，并在操作开始/完成时刷新两个入口；菜单栏在 busy
期间每秒采样，避免恢复后长时间保留过渡状态。

Doctor 包含真实 `mihomo -t`，单次配置验证最长可到 90 秒，因此不得从
`/api/v1/overview`、`/api/v1/menubar`、SSE 或其他轮询热路径同步执行。Web 诊断页通过
`POST /api/v1/doctor` 显式启动 Control Service 内的 single-flight 后台检查，再用
`GET /api/v1/doctor` 读取运行状态和缓存结果；重复请求只能观察同一份进行中的任务。
缓存以主配置、设备策略与 imported profile 摘要共同标识，配置变化后只能显示为旧结果，
不能继续影响当前菜单栏健康状态。这个只读 Doctor 缓存不参与 start/reload 放行；两者仍须
执行各自的真实预检与 TUN readiness，不能用历史 Doctor 成功结果替代。

菜单栏与 Web GUI 总览用“IPv4 接管”和“IPv6 接管”展示按地址族归一化后的运行状态，
不能把 raw forwarding 直接改名成 IPv4 接管。IPv4 只有在当前 boot 的 gateway runtime
active、PF anchor loaded、forwarding enabled 且整体 gateway running 时才显示正在接管；
停止态即使宿主原本已经启用 forwarding 也显示已停止。IPv6 接管来自用户态 packet path，
区分正在接管、自动模式等待上游、已关闭、已停止、异常与重启后待清理。底层
`forwarding`、`ipv6_packet`、`native_ipv6_available` 和 `ipv6_reason` 继续保留用于诊断。

## 语言与共享控件

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

Main UI selects share the themed select-only combobox in `Select.tsx`, including
portal positioning, keyboard/typeahead navigation, cancellation, disabled options
and explicit WebKit pointer focus. Scrollbars and text selection share light/dark
control colours. The page canvas extends its background through the transparent
scrollbar track; thumbs use a low-opacity neutral green, including on hover.
Language saves fence background preference refreshes until the
mutation completes; async catalog preparation must recheck the request generation
before updating the rendered language.

## 网关操作与网络设置

Web GUI 总览页的“启动网关”与“停止网关”只导航到 `network` 页面，不得直接调用
gateway start/stop API。真实生命周期动作留在网络页，使 topology、plan blocker、DHCP
接管与恢复状态在用户确认前保持可见。“启动网关”只切换页面，不改变当前滚动位置；
“停止网关”切换页面后滚动到页面底部，完整露出恢复状态机的当前操作按钮。

网络页对 `same_wifi_dhcp` 保留带恢复证据的完整状态机；`same_lan` 与 `isolated_lan`
使用独立“网关运行控制”卡片直接调用 start/stop operation。未保存配置必须阻止启动，
但不能阻止运行中网关的安全停止；`degraded` 仍属于运行中状态。两种直接启停路径都要
显示 topology 对应的影响范围并要求显式确认。

配置填写提示应作为表单内的低强调步骤说明，保存区与最后一组字段保持明确间距，并显示
当前已保存或存在未保存修改，避免按钮紧贴字段卡片。设备页的未保存修改与已保存待重载
状态使用同一套底部浮动操作条；保存后直接从“保存设备配置”切换为“应用并重载网关”，
不把待重载提示移回页面顶部。

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

## 设备工作台

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
主界面不把 Profile 作为复用对象：“设备与规则”默认进入设备工作台，左侧清单与右侧
身份、路由和分流草稿共用同一个设备选择。规则集和无出口分流模版各有独立页签。
清单合并 desired/applied，保留待移除、暂停及网段外登记；搜索同时覆盖新旧身份，筛选
只影响展示。切换设备或页签不写配置，草稿保存与 reload 仍是两个明确阶段。
身份细节可折叠，身份冲突、地址变化、出口调整与待应用状态保持可见。
当前 Mac 设置独立显示；右侧即时出口来自 applied，分流顺序和候选编辑来自 desired。
设备出口选择在卡片内展开；新增候选入口定位到对应 desired 编辑区，仍需保存与 reload。
规则出口名称使用 applied 编译结果的 `rule_matches` 元数据，按模版、规则集或匹配条件
显示，不能从同 ID 的 desired 规则猜测 applied 名称。只编辑候选时保留原规则的组合匹配条件。

设备卡自身提供整设备管理：「编辑身份与路由」复用登记面板并预填现有身份，「删除设备」
同时清理该设备的私有 Profile。两者都只改本地草稿，仍走同一次“保存设备配置”。设备
policy 是整文档提交，因此这些入口是操作者修正身份的唯一途径；`out_of_lan_devices`
标记的设备只能在这里改地址或删除，界面必须把这条出路说清楚。

设备策略模型、身份暂停与出口失效规则见 [设备策略契约](device-policy-overlays.md)。revision 冲突保留本地草稿；selector API 从设备 ID 与 slot 重建组名，拒绝伪造任意 group。只有与 applied reservation 完全一致且未过期的当前 lease 才提供 DHCP 身份就绪证据。

## 策略、准备态与连通性

策略页使用 [profile overlay 的 prepared workspace](mihomo-profile-overlay.md#Prepared-policy-workspace)，运行中读取 applied core，停止时读取无网络接管的准备态核心。策略页是预览、选择与检测入口，App 启动复用同一候选合成；普通 CLI 只启动持久化 desired。

来源、附加配置或 Tailscale 任一为空时返回正常的空/部分策略快照；Provider 首次加载
尚无节点也属于正常空状态。候选错误必须显示具体原因并清除旧成功快照，不能回退到旧
配置启动。

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

## 其他领域入口

- 临时合盖运行、Helper lease 与所有权：[睡眠接管契约](lid-closed-sleep-prevention.md)。UI 成功后立即采用返回状态，较早读取不能覆盖较新的 PUT；耗电、发热和通风提示必须保留。
- 连接表、流量归属、采样与显式刷新：[连接观察契约](connection-observation.md)。
- 接管阶段、恢复卡与用户确认：[DHCP 恢复契约](dhcp-recovery.md)。
- 下游 IPv6 设置与共享 L2 确认：[IPv6 契约](downstream-ipv6-takeover.md)。
- 浏览器会话、配置 revision 和来源文件操作：[Control API](../control-api.md)。
