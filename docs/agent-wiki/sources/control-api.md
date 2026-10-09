# Control API 与配置应用

本页维护本地 API 的认证、存储、配置事务、进度与诊断接口。原生 relay 的凭据边界见 [桌面宿主](decisions/desktop-host.md)，展示与交互规则见 [GUI 控制面](decisions/gui-control-plane.md)。

## 服务与权限

- `cmd/opensurge-control` 是只监听 `127.0.0.1` 的 Go Control API，并嵌入
  `web/` 构建出的 React 应用；
- `apps/desktop/` 是 Wails + 系统 WebView 宿主，主窗口与菜单栏面板都复用
  React/TypeScript；认证、窗口与固定原生能力见 [Desktop host](decisions/desktop-host.md)；
- `cmd/opensurge-helper` 是窄权限 Unix-socket helper，只接受 start/stop、网络
  固定地址/恢复 DHCP 和主动 DHCP OFFER 探测等固定动作，不提供任意 shell 命令；
  生产配置、runtime 和可执行文件必须位于允许目录、由 root 拥有且不能被普通用户修改。

Control API 默认监听 `127.0.0.1:61767`。用户 store 为 `~/Library/Application Support/OpenSurge/`，系统 applied 配置与 runtime 属于 root；普通控制服务经 Helper 的固定动作修改系统状态。

## 浏览器会话

控制服务会输出一个 30 秒有效、一次性的 bootstrap URL，浏览器用它换取 HttpOnly
session。Wails 通过 native relay 执行同样的交换，token 和 cookie 保留在原生内存，不进入
JavaScript。正式安装位置的 App 启动时唤醒已有用户 Control Service；显式重新连接同样
不使用 `kickstart -k`，不会强制重启后台或启动网关。Preview 启动不操作服务。

Control API 的 bootstrap `expires_at` 来自 Go `time.Time`，可能包含 RFC3339 小数秒；
菜单栏客户端必须同时接受带小数秒和不带小数秒的时间格式，不能把该解码失败误判为浏览器
打开失败。

Web GUI 从一次性 bootstrap 链接换取的 HttpOnly 会话使用 12 小时闲置期限；每次有效的
浏览器会话请求都会滑动续期，因此持续打开的控制面不能在固定时间点突然失效。会话仍只
保存在 Control Service 内存中，不跨服务重启持久化。浏览器收到 401 后必须停止 API 轮询
与 SSE，并明确引导用户点击 macOS 菜单栏中的 OpenSurge 图标，再选择“在浏览器中打开”；
不能继续提供必然再次返回 401 的普通“重试”按钮。

菜单栏的 Control API bearer token 只从用户应用支持目录内权限为 `0600` 的
`control-token` 读取，不复制到 Keychain，也不回退到可能过期的旧 Keychain 副本。文件
缺失与 endpoint 尚未生成都表示用户级 Control Service 尚未准备好：先轻量 kickstart 并
重试，仍失败才显示友好错误和“重新连接”。v0.3 的启动与显式重新连接都不使用
`kickstart -k`，只唤醒已有服务，不强制重启 Control Service 或操作网关数据面。
Preview 默认不管理已安装服务；旧 Swift 的重启实现只供历史版本维护。

## 来源、凭据与应用

Web GUI 包含总览、网络设置、来源、设备、策略、连通性和诊断。来源支持本地 YAML 与 HTTPS
URL，先保存 SHA-256 标识的只读快照，再检查 profile inventory 和保留命名空间；应用
时使用真实 mihomo `-t` 验证 overlay。URL 获取拒绝 loopback、私网、链路本地地址、
非 HTTPS 重定向、超过三次重定向和超过 10 MiB 的响应。

同一 source 的刷新保留旧版本 metadata、digest、inventory 和 applied 标记，新内容保持
为未应用草稿，并展示 proxies/groups/providers 与规则数 diff。来源状态由 desired config
中的 profile 内容 digest 与 runtime state 的 applied profile digest 推导，不能只凭“写过
config”就标记为运行版本。网关运行时，显式应用会先验证完整候选，再执行完整 reload；
只有 reload 成功并重写 `runtime/mihomo.yaml` 后才替换运行版本。网关停止时只保存为“下次
启动版本”。reload 失败会原子恢复旧 config；若新启动已经清掉旧 state，还会尝试用旧配置
恢复网关，界面继续显示旧 applied 与新 desired 的真实状态。

带 token/query/basic-auth 的完整刷新 URL 存入用户应用支持目录下独立的
`credentials/sources.json`：目录权限为 `0700`、文件权限为 `0600`。它不进入公开的
sources JSON、API 响应、诊断、日志或界面；公开 `origin` 会移除 userinfo、query 和
fragment。来源快照仍是用户目录下权限为 `0600` 的按 digest 版本文件。升级时只尝试一次
从旧 `com.opensurge.sources` Keychain 项迁移；成功或失败都会写入迁移标记，避免后续启动
继续访问 Keychain 或反复触发授权。旧 Keychain 项不自动删除；迁移失败不阻止 Control
Service 启动，已有快照仍可使用，刷新地址可通过重新导入补回。

HTTPS source 请求使用 mihomo/Clash Meta 兼容的 User-Agent，因为部分订阅服务会按
客户端标识选择响应格式。草稿只做结构校验；apply 只由 privileged helper 对最终候选
执行一次真实 `mihomo -t`。生成配置把 geodata 下载指向 MetaCubeX 官方仓库列出的
JSDelivr-CF 入口，下载结果保存在 applied profile 所在数据目录供后续校验与启动复用。

运行中 source apply 必须写入一条 reload operation 供诊断审计；start/reload 的网络顺序与失败恢复见 [网关生命周期](decisions/gateway-lifecycle.md)。

来源列表只公开经过管理目录校验的 `snapshot_display_path`，继续清空原始
`snapshot_path` 与订阅取回地址。复制完整路径、Finder 定位和导出副本分别通过受认证的
来源文件接口执行；动作前必须同时校验 source ID、digest、规范化路径、私有权限和文件
内容摘要，不能把任意持久化路径交给 Finder。管理快照只读，导出副本写入 Control Service
用户数据目录下的 `exports/`，目录权限 `0700`、文件权限 `0600`，命名包含来源名、摘要短值
和时间戳且不得覆盖旧文件。导出成功后用用户会话中的 Finder 选中新文件；如果 Finder
调用失败，已经写出的副本必须保留，并在错误反馈中给出其显示路径。

## 配置与发现

网络配置通过 revisioned `GET/PUT /api/v1/config` 修改，写入要求 `If-Match`；只允许 topology、DHCP/DNS、
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

## 操作进度、重试与 SSE

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

`/api/v1/events` 每两秒观察 config、gateway、device-policy 与 profile 的 desired/applied
digest 以及 recovery，只有
状态变化时发送 `state` SSE，另有 15 秒 heartbeat。诊断页通过受认证接口显示 live
connections 与最多 80 行近期日志；已知 mihomo/upstream 凭据在 API 返回前脱敏。
诊断 DTO 同时带最近 20 条持久化 start/stop/reload operation 与当前 recovery 状态，另有
`GET /api/v1/operations` 返回最近 50 条，便于审计幂等 operation ID、失败和完成时间。

## CLI 诊断入口

`omg` CLI 仍是受支持的运维、诊断、自动化和恢复接口。`status`、`doctor`、
`leases`、`logs`、`policies`、`local-routing`、`devices`、`connections`、
`providers`、`provider-update` 和 `snapshot` 都有机器可读 JSON 形态；
`logs --tail N --format json` 会返回最近的 dnsmasq/mihomo 日志行，并对每个日志
文件标出存在状态和读取错误。`snapshot --format json` 聚合 status、doctor、
leases、日志尾部、策略组、连接和 provider 状态，并把 mihomo API 不可用记录在
局部字段里，供 GUI 后端与自动化诊断复用。
`start --format json` 和 `stop --format json` 在动作成功后返回结构化成功 payload；
失败仍保留非零退出码，并在 `--format json` 时把
`{"command":"...","ok":false,"error":"..."}` 写到 stderr。

## 相邻规则

来源合成与准备态工作区见 [profile overlay](decisions/mihomo-profile-overlay.md)；连接与归属接口见 [连接观察](decisions/connection-observation.md)；DHCP 恢复接口见 [恢复契约](decisions/dhcp-recovery.md)。
