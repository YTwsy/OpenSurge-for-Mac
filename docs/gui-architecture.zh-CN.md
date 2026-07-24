# Web GUI 与菜单栏 App

OpenSurge 现在提供同一个本地控制面之上的两个 GUI 入口：

- `cmd/opensurge-control` 是只监听 `127.0.0.1` 的 Go Control API，并嵌入
  `web/` 构建出的 React 应用；
- `apps/menubar/` 是 macOS 13+ 的原生菜单栏 App，由 AppKit 状态项承载 SwiftUI
  状态面板，只显示状态、恢复警报并打开 Web GUI；
- `cmd/opensurge-helper` 是窄权限 Unix-socket helper，只接受 start/stop、网络
  固定地址/恢复 DHCP 和主动 DHCP OFFER 探测等固定动作，不提供任意 shell 命令；
  生产配置、runtime 和可执行文件必须位于允许目录、由 root 拥有且不能被普通用户修改。

## 开发运行

```sh
make web-install
make control-build
./bin/opensurge-control --config examples/config.example.yaml
```

控制服务会输出一个 30 秒内有效、只能使用一次的 bootstrap URL。浏览器通过它换取
`HttpOnly`、`SameSite=Strict` session cookie。API mutation 还会校验 Origin；菜单栏
App 使用应用支持目录内权限为 `0600` 的本地 token 请求一个新的短期 bootstrap URL，
不会把长期凭据放进浏览器历史，也不会把该 token 再复制到 Keychain。该 token 的事实
来源始终是用户级 Control Service；文件尚未生成时，菜单栏自动唤醒服务并重试，仍失败
才显示“OpenSurge 后台服务尚未准备好”和显式“重新连接”。用户点击重新连接只会重启
用户级 Control Service，不会停止正在运行的网关数据面。

菜单栏状态面板由 AppKit `NSStatusItem` 与锚定的 `NSPopover` 承载，内部继续复用
SwiftUI `MenuContentView`。点击菜单栏图标会切换面板；用户首次主动打开或再次打开
`/Applications/OpenSurge.app` 时，`NSApplicationDelegate` 会要求同一个控制器确保
面板已经展开。每次菜单栏 App 进程启动完成都会主动展开面板，因此通过“登录时显示”
启动时也会弹出。

菜单栏提供两个不同的退出层级。“只退出菜单栏 App”在二次确认后直接结束菜单栏进程，
不会改变用户级 Control Service、网关数据面或 root Helper。“退出 OpenSurge”只有在
Gateway、DHCP/DNS、mihomo、PF 与 IPv4 forwarding 已确认停止，并且没有待处理网络恢复时
才可执行；它会 bootout 用户级 `com.opensurge.control` 并退出菜单栏 App。已安装的
`com.opensurge.helper` 继续由系统 launchd 托管并保持空闲加载，不卸载、不要求下次打开
再次授权。重新打开菜单栏 App 时，如果用户级 Control Service 已被 bootout，App 会从
现有 LaunchAgent plist 重新 bootstrap 并连接它。
退出确认使用同步的 AppKit `NSAlert.runModal()` 并直接消费返回值，不再把进程退出动作
依赖于 SwiftUI alert 的状态写回/关闭时序。早期 `MenuBarExtra` 的 SwiftUI alert 曾在两个退出
入口中都只关闭确认框而没有进入动作回调，因此这里保留显式的 AppKit 边界。

Control API 默认位置是 `http://127.0.0.1:61767`。端点描述、token、来源快照、操作记录
和局域网 DHCP 接管恢复状态位于：

```text
~/Library/Application Support/OpenSurge/
```

开发环境若确实需要执行网关动作，可用 root 身份运行 control service 并显式传入
`--direct-root`。普通开发和 UI 验证不要使用这个参数；未安装 helper 时 start/stop
会返回结构化错误，不会尝试提权或弹出不透明的 shell。

## Web 页面

Web GUI 包含总览、网络设置、来源、设备、策略、连通性和诊断。来源支持本地 YAML 与 HTTPS
URL，先保存 SHA-256 标识的只读快照，再检查 profile inventory 和保留命名空间；应用
时使用真实 mihomo `-t` 验证 overlay。URL 获取拒绝 loopback、私网、链路本地地址、
非 HTTPS 重定向、超过三次重定向和超过 10 MiB 的响应。

总览的 GATEWAY 卡片是网关身份和配置上下文的唯一入口：接口、LAN IPv4、当前配置的
topology 与 desired/applied 状态都在卡内展示，不另设重复的信息条。`GET /api/v1/overview`
直接返回当前配置的 `topology`，不能用只在恢复流程中落盘的 `recovery.topology` 代替。
总览页标题区的“启动网关”或“停止网关”只是进入“网络设置”的上下文入口，不直接调用
gateway start/stop API；实际动作必须留在网络页，让用户先看到 topology、计划 blocker、
DHCP 接管与恢复状态后再确认。
上传、下载和总趋势都使用平滑曲线；新采样到达时速率数字立即采用新值，曲线点则在约
700 ms 内做缓出插值，60 秒窗口滚动时不会整条折线瞬间跳变；系统启用“减少动态效果”
时曲线也直接采用新值。
网关总趋势采用较矮的紧凑卡片和单条低对比参考线，
避免把 60 秒内存样本误做成大面积历史报表。

同一 source 的刷新保留旧版本 metadata、digest、inventory 和 applied 标记，新内容保持
为未应用草稿，并展示 proxies/groups/providers 与规则数 diff。来源状态由 desired config
中的 profile 内容 digest 与 runtime state 的 applied profile digest 推导，不能只凭“写过
config”就标记为运行版本。网关运行时，显式应用会先验证完整候选，再执行完整 reload；
只有 reload 成功并重写 `runtime/mihomo.yaml` 后才替换运行版本。网关停止时只保存为“下次
启动版本”。reload 失败会原子恢复旧 config；若新启动已经清掉旧 state，还会尝试用旧配置
恢复网关，界面继续显示旧 applied 与新 desired 的真实状态。

导入 URL/文件、刷新草稿以及完整校验和重载都必须在触发按钮上显示进行中状态，并在
完成后保留明确成功或错误反馈；不能只用全页 disabled 让用户猜测动作是否已经受理。

带 token/query/basic-auth 的完整刷新 URL 存入用户应用支持目录下独立的
`credentials/sources.json`：目录权限为 `0700`、文件权限为 `0600`。它不进入公开的
sources JSON、API 响应、诊断、日志或界面；公开 `origin` 会移除 userinfo、query 和
fragment。来源快照仍是用户目录下权限为 `0600` 的按 digest 版本文件。升级时只尝试一次
从旧 `com.opensurge.sources` Keychain 项迁移；成功或失败都会写入迁移标记，避免后续启动
继续访问 Keychain 或反复触发授权。旧 Keychain 项不自动删除；迁移失败不阻止 Control
Service 启动，已有快照仍可使用，刷新地址可通过重新导入补回。

`GET/PUT /api/v1/config` 只暴露 topology、DHCP/DNS、TUN 与 device-policy 开关等
非敏感字段，并强制 `If-Match` revision。生产环境由 helper 原子写入 root-owned config；
网关运行或恢复未完成时拒绝 topology 修改。网络页可切换 `same_lan`、
`same_wifi_dhcp`、`isolated_lan`，并可初始化空 device-policy 文件。

设备页把 applied 与 desired 持续分为两层：顶部绿色“即时生效”只切换已经应用的全局或
`device/<id>/<slot>` selector；下方黄色“保存后重载”才编辑设备身份、selector 成员和
规则。`THIS MAC` 只列出非 `device/` 的既有全局组，并明确它只影响当前规则引用该组的
流量，不代表全部 Mac 流量、未匹配流量或 macOS 系统代理。

普通登记默认创建 `<device-id>-policy` 私有 Profile。设备首次从主路径修改共享 Profile
或继承 Template 的 Profile 时，前端把解析后的有效候选与规则复制到无 Template 的私有
Profile，并只更新该设备引用；不修改 `PolicySet` schema。高级区仍保留 Profiles、
Templates 和 Rule Sets，被引用对象禁止删除并显示引用来源。主规则表单使用 chips 和
候选选择，不要求逗号分隔字符串；revision 冲突保留本地草稿。

selector API 根据设备 ID 和 slot 重建 `device/<id>/<slot>`，不会接受调用方直接伪造任意
group 名。保存由 helper 使用当前 imported inventory 与真实 mihomo 校验候选，不只做
JSON 结构检查。`GET /api/v1/devices` 同时返回 desired/applied 设备与解析后内容变化的
设备 ID，供界面精确显示“已应用、待应用、待更新、待移除”；身份就绪还要求在线且未
过期的 lease MAC、IPv4 与 applied reservation 完全一致。

策略页是完整节点健康中心。`GET /api/v1/proxy-health` 从 mihomo `/proxies` 读取节点
类型、UDP 能力、provider、当前链路和最近 history；`POST /api/v1/proxy-health/tests`
只接受当前可探测的节点名称，并通过 mihomo 单节点 delay API 使用固定检测 URL。页面
集中显示延迟/超时/不可达并允许 Selector 即时切换；设备页只保留当前出口摘要，展开
选择器时才显示候选健康。这里的延迟是网关 Mac 到检测地址的节点探测，不是设备端到端
验收。

连通性页使用后端固定目录，`POST /api/v1/connectivity/tests` 由 Control Service 通过
当前 runtime mixed-port 对每个真实站点进行三轮请求，展示中位延迟，并尝试从活动
connections 采集命中规则、payload 和完整出口 chain。它只证明 applied 全局 mihomo
路径，不能伪装成某台下游设备的 `SRC-IP`、DHCP、DNS 或 TUN 证据。Net.Coffee 以外链
方式保留为浏览器本机线路检测，并与网关策略路径明确分栏；前端不会代理或嵌入第三方
页面，也不会把浏览器结果归因到 OpenSurge 网关。

`POST /api/v1/gateway/reload`、`omg reload` 和运行中来源应用共用 operation/audit 与
privileged helper
allowlist。Reload 只允许健康运行的网关；先在临时 runtime 做完整静态校验、接口/LAN、
protected/reservation 冲突和真实 `mihomo -t`，失败不触发 stop。通过后执行完整 stop/start，
成功写入新 applied device-policy snapshot/digest 和 applied profile digest。same-LAN 成功保留 `gateway_active`、
`client_validated` 或 `client_validation_skipped`；stop 失败保留原 recovery，stop 成功但 restart 失败降回
`router_dhcp_disabled_confirmed`，让用户直接重试启动或进入恢复。它不承诺零中断。

`/api/v1/events` 每两秒观察 config、gateway、device-policy 与 profile 的 desired/applied
digest 以及 recovery，只有
状态变化时发送 `state` SSE，另有 15 秒 heartbeat。诊断页通过受认证接口显示 live
connections 与最多 80 行近期日志；已知 mihomo/upstream 凭据在 API 返回前脱敏。
诊断 DTO 同时带最近 20 条持久化 start/stop/reload operation 与当前 recovery 状态，另有
`GET /api/v1/operations` 返回最近 50 条，便于审计幂等 operation ID、失败和完成时间。

总览的设备流量面板每 2 秒读取受认证的 `GET /api/v1/device-traffic`。Control Service
用 DHCP lease、applied 静态设备和当前观察到的网关 LAN IPv4 建立下游设备清单，再按
mihomo connection 的 `metadata.sourceIP` 归属当前活跃会话 `upload`/`download`。带有
本机 process/processPath 证据、来自回环/网关地址或与这些证据共享源地址的连接单独聚合
到 `gateway_local`，不混入 `devices` 或下游设备合计；GUI 始终把“本机 Mac”固定为
“活跃设备”的第一行，并按实际 connection type 显示 TUN、显式代理或两者。没有 DHCP/
静态身份、但能确认网关 LAN 源 IPv4 的会话进入 `observed_traffic` 行并计入
`unidentified_device_connections`；地址缺失、网段外且没有本机证据等剩余连接进入
`unclassified_connections`，只作为诊断提示。`unmatched_connections` 仅作为旧客户端
兼容字段保留，当前 GUI 不再用它解释来源身份。

主出口选择当前会话累计字节最多的完整 `chains`，相同字节时再按连接数和名称稳定决胜。
该 DTO 明确标记 `scope=active_sessions`，不表示重启后仍保留的历史流量。若 mihomo 不可用，
接口仍返回本机行、DHCP/静态设备清单和 `connection_error`，GUI 不把零流量误报为持久化
统计。Control Service 还在内存中按 connection ID 比较相邻计数器，生成网关整体、本机
和每设备的 bytes/s；首次采样、新连接、采样间隔超过 15 秒或 mihomo 读取失败后先重建
基线，避免把会话累计值误算成瞬时速度。总览只保留最近 60 秒内存趋势，不冒充今日或
月度持久化历史。点击本机或设备行时，设备列表收拢并在右侧显示使用同一图表组件的趋势；
设备 IPv4 列在展开状态仍然保留。宽屏右侧趋势卡绝对定位在由设备列表决定的网格行内，
不参与行高计算，因此展开和收起不会改变页面总高度或触发底部滚动跳动；窄屏才改为带
高度过渡的纵向展开。

## 局域网 DHCP 接管恢复状态

`/api/v1/recovery` 持久化以下受验证状态机：

```text
idle -> prepared -> mac_static -> router_dhcp_disabled_confirmed
     -> gateway_active -> client_validated | client_validation_skipped
     -> gateway_stopped_waiting_router_dhcp
        -> router_dhcp_restored -> complete | complete_static
        -> complete_static
```

当配置为 `same_wifi_dhcp` 时，Control API 在没有
`router_dhcp_disabled_confirmed` 收据时拒绝 start。确认收据来自 root helper 的主动
DHCPDISCOVER：仍收到任何 OFFER 就硬阻塞。成功 stop 后状态进入
`gateway_stopped_waiting_router_dhcp`。推荐路径重新探测到路由器 OFFER 后再把 Mac 恢复
为自动 DHCP；启动期 plan blockers 不得禁用停止后的恢复动作。若主动 OFFER 探测不可用，
用户可在明确确认路由器 DHCP 已恢复并接受断网风险后，使用人工兜底跳过 OFFER 证据；
该动作仍经 root helper 真实恢复 Mac 自动 DHCP，成功后才写入 `complete`，不能只跳状态。
如果用户明确要长期保持静态 IPv4，也可在网关已成功停止后从
`gateway_stopped_waiting_router_dhcp` 或 `router_dhcp_restored` 进入 `complete_static`。这条
路径不调用 DHCP OFFER 探测或 `SetDHCP`，必须警告路由器 DHCP 与其他客户端的自动获取
能力没有被验证或恢复。
`gateway_active` / `client_validated` / `client_validation_skipped` 是预期的接管运行态，显示正常运行或
验收状态；恢复警报仅用于启动前的中断状态和 stop 后等待路由器/Mac DHCP 恢复的阶段。
状态机不会自动修改未知路由器，
也不能把同一二层 LAN 描述为不可绕过隔离。现有配置枚举仍保留
`same_wifi_dhcp` 以兼容已安装配置，但产品 UI 使用“局域网 DHCP 接管”，因为承载
介质既可以是 Wi-Fi，也可以是以太网。

`prepared` 后网络页会直接展示原始 IPv4、路由器、DNS、网络服务、接口与掩码，并通过
`GET /api/v1/recovery/card` 查看或下载中文离线恢复卡。此阶段尚未改变网络，用户可以用
`POST /api/v1/recovery/discard` 销毁恢复资料并回到 `idle`；进入 `mac_static` 后该动作会被
拒绝。路由器地址是有效 IPv4 时，界面提供 HTTP 管理页链接；关闭与恢复 DHCP 两个阶段
都显示通用 LAN / 网络设置 / DHCP 服务器操作路径及无法自动发现路由器时的 fallback。

Web GUI 的侧边栏提供浅色 / 深色主题切换，选择保存在浏览器本地存储中，不进入 Control
API 配置或 root-owned gateway 配置。

启动后推荐先输入验收客户端 IPv4，后端要求活跃租约、
DHCPACK、该源 IP 的 DNS 查询和 mihomo TUN 日志，同时操作者确认客户端网关/DNS 指向
Mac 且无显式代理；若快照存在 IPv6 default，还必须确认绕过警告。紧急 stop API 始终
可用，不会因验收失败阻碍恢复。Web GUI 也提供显式“跳过客户端验收”：它写入
`client_validation_skipped` 和持久化说明后允许正常停止，但必须明确本次运行没有客户端
DHCP、DNS 或 TUN 验收证据，不能显示成“已验收”。

## 菜单栏边界

菜单栏 App 不提供 start/stop、provider refresh 或策略切换。它每 15 秒获取一次
`/api/v1/menubar`，窗口打开时每 2 秒刷新，失败时指数退避到最多 60 秒，并根据 connecting、
stopped、running、degraded、recovery、unreachable 显示状态。首次启动尚未取得状态时，
connecting 从第一帧就使用半透明 OpenSurge 品牌图标；只有真实请求失败后，unreachable 才
使用更低透明度和对应无障碍文案表示不可达，但仍保持品牌图标，不再显示看起来像旧版图标
的 `network.slash`。恢复警报优先于其他状态。
网关明确处于 `stopped` 时显示“OpenSurge 网关已停止”；此时 runtime-oriented doctor
未通过或存在待应用配置都不能把“未启动”误报成“运行异常”。
退出按钮只终止菜单栏 App；点击后会先提示后台 Control Service 仍会继续，若网关正在
运行，还会明确 DHCP/DNS、mihomo、PF/转发不会随菜单栏退出。停止网关仍须进入 Web GUI。

构建：

```sh
make menubar-build
./scripts/build-menubar-app.sh
```

构建统一 pkg：

```sh
OPENSURGE_MIHOMO_BINARY=/path/to/mihomo \
OPENSURGE_DNSMASQ_BINARY=/path/to/dnsmasq \
OPENSURGE_VERSION=0.1.1 \
OPENSURGE_BUILD_NUMBER=2 \
make gui-installer
```

`OPENSURGE_VERSION` 同时写入 pkg receipt 和菜单栏 App 的
`CFBundleShortVersionString`，`OPENSURGE_BUILD_NUMBER` 写入 `CFBundleVersion`；不要让新
pkg 携带仍标成旧版本的 App，否则现场无法可靠区分已安装二进制是否包含最新修复。

安装器显式以 `/` 为 payload 根目录，并将 `OpenSurge.app` 声明为不可
relocatable bundle，确保它固定安装到 `/Applications/OpenSurge.app`。
不要移除 `packaging/gui-components.plist` 或改回让 `pkgbuild` 自动推断安装位置；
否则 Installer 可能把 App relocate 回构建工作区的 `payload/Applications`。

升级到采用产品名 bundle 的版本时，postinstall 会在新 payload 已落盘后删除旧的
`/Applications/OpenSurge Menu Bar.app`，避免 Launchpad 同时保留两个相同 bundle ID
的入口；卸载脚本兼容清理新旧两个路径。内部 Swift executable 仍名为
`OpenSurgeMenuBar`，launchd label 与 bundle identifier 也不因这个显示名称迁移而改变。

安装包包含 Web 静态资源（嵌入 control binary）、用户级 Control Service、菜单栏 App、
CLI 和 root helper。postinstall 会创建 root:admin、用户只读的 applied 配置/runtime，
安装固定 launchd 服务；卸载脚本在 recovery 未完成时拒绝删除，并先停止网关。

升级采用与安全卸载一致的前置顺序：preinstall 先检查 recovery 只能处于 `idle`、
`complete` 或明确保留静态 IPv4 的终态 `complete_static`，再 bootout 用户级 Control
Service 并退出菜单栏 App，调用旧版本
`omg stop` 清理 DHCP/DNS/mihomo/pf/forwarding，最后 bootout root helper。任何一步
失败都在 Installer 覆盖旧 payload 前终止。postinstall 只在首次安装时 seed
`config.yaml`，升级保留现有 config、`data/` 与 `runtime/`。

正式发布可设置 `OPENSURGE_CODESIGN_IDENTITY` 和 `OPENSURGE_INSTALLER_IDENTITY` 后构建，
再设置已由 `notarytool store-credentials` 创建的 `OPENSURGE_NOTARY_PROFILE`：

```sh
make gui-notarize PKG=artifacts/gui-installer/OpenSurge-for-Mac-0.1.0.pkg
```

缺少 Apple Developer 身份时，GitHub tag workflow 会发布文件名明确带
`arm64-unsigned.pkg` 与 `x86_64-unsigned.pkg` 的正式 Release，并同时提供合并的 SHA-256
清单与各安装包的 GitHub artifact attestation。GitHub 的正式 Release 状态不等于 Developer
ID 签名或 notarization，不能宣称安装包可被 Gatekeeper 直接放行。用户安装说明只使用系统
设置中针对该包的“仍要打开”，不得要求全局关闭 Gatekeeper 或递归移除 quarantine。
当前安装器 seed 配置必须使用 managed profile 且不引用外部 device-policy；订阅与设备
策略在安装后经控制面导入，避免 pkg 携带工作区绝对路径。

## 验证

```sh
make test
make web-build
make menubar-build
make menubar-test
```

`menubar-test` 使用独立 Swift 检查程序与 mock `URLProtocol`，因此只安装 Command Line
Tools 也可验证 icon 优先级、Bearer、最小 DTO、bootstrap 深链接、URL 打开失败回退、
Control Service transport 恢复、两级退出门禁、root Helper 不被 bootout 和 token 泄漏边界。
`apps/menubar/Tests` 仍保留 XCTest 版本，供具有完整 Xcode/XCTest 的 CI 执行。

修改真实网关、TUN、DHCP 或设备策略数据面后，仍须运行对应的 lab/same-WiFi gate；
GUI 构建通过不能替代这些 host-network 证据。
