# 每设备策略契约

本页维护设备身份、路由模式、匹配、编译与失效回退。操作者的配置说明见 [设备策略指南](../../../user/device-policy.zh-CN.md)。

OpenSurge 只运行一个 mihomo。`device_policy.file` JSON 文件为每台设备记录
固定 IPv4、可选 MAC 与 profile；编译时按拓扑将它们转换为 DHCP reservation、独立 selector
group，以及以 `SRC-IP-CIDR` 区分来源的 mihomo 规则。它不是“一台设备一份完整
mihomo YAML”。

安装版始终启用每设备策略：新安装初始化空文件；升级时只为未配置路径的旧配置
补齐默认路径，已有自定义路径和文件内容保持原样。Web GUI 不再显示启用开关，
网络配置保存忽略旧客户端的 `device_policy.enabled=false`，并保留受保护地址设置。
若默认路径已存在此前停用时留下的文件，应重新使用并校验它，禁止用空 starter
覆盖。保存失败只允许回收本次新建的文件，不能删除旧策略。独立 CLI 配置仍支持
显式文件路径，配置读取及 stop/status 不承担文件初始化或迁移副作用。

## 策略模型

- 每台设备必须明确选择 `egress_mode`：`inherit_global` 只保留设备覆盖，未命中流量继续
  走 imported/managed 网关规则，不跟随 Mac 本机 Rule/Global/Direct 开关；
  `dedicated` 为公网流量生成并优先使用 `device/<id>/default` selector。
- `dedicated` 在设备覆盖和默认 selector 之前生成按设备源 IPv4 限定的本地/私网、
  link-local、CGNAT 与 multicast `DIRECT` 保护，避免远端代理吞掉 LAN 访问。
- 含 `policies` 的设备规则会获得 `device/<id>/<rule-id>` selector；
  `device-policy-select` 只能选择此设备拥有的 selector。
- 含 `action` 的规则直接发往 `DIRECT`、`REJECT` 或已有全局 mihomo group。
- `domains`、`ip_cidrs`、`protocols`、`ports` 和 `rule_sets` 可组合；字段之间为
  AND，同字段多个值为 OR。`match.template` 是互斥的简写，按声明顺序
  展开模版的 `rule_sets`。
- 面向用户的分流模版只组合 `rule_sets`，不携带默认出口或命中出口；出口始终
  位于具体设备分流的 `action` 或 `policies`上。Profile 仍是编译和持久化用的内部容器。
- `gateway_target` 默认为 `opensurge`。只有 `same_wifi_dhcp` 可选
  `upstream_router`：保留 MAC 固定租约，但 dnsmasq 通过 tag 向该客户端下发
  `dhcp.bypass_gateway` 和 `dhcp.bypass_dns`。这是 IPv4-only 绕行：编译结果不为其
  生成代理 selector/普通设备规则，但必须保留 Profile、规则和 `egress_mode`，切回后
  恢复。若下游 IPv6 开启，仍保留 MAC→`device:<id>` 映射，只在所有其他规则之前生成
  `AND,((IN-TYPE,TUN),(IN-USER,device:<id>)),REJECT`；其他设备 IPv6 不受影响。
  切换是 save-and-reload，且只有客户端 DHCP 续租/重连后新 IPv4 Router/DNS 才生效。
  设备可能仍有 SLAAC/RDNSS，控制面只能写“IPv6 出站已阻止”。共享 L2 必须关闭主路由
  RA/DHCPv6 或使用 RA Guard，否则 IPv6 会绕过 OpenSurge。

Web GUI 普通登记默认选择 `inherit_global`，路由模式变化需要保存与重载；即时 default
selector 只展示已应用的 dedicated/legacy 设备。界面向用户暴露规则集、无出口分流模版和
设备分流；Profile 是内部持久化与编译容器。
登记创建 `<device-id>-policy` 私有 Profile；首次编辑共享或旧式继承 Profile 时，把有效内容
私有化到该设备。设备工作台的布局、desired/applied 展示与操作入口见
[GUI 控制面契约](gui-control-plane.md)。

规则库预置一份可阅读的 Claude Code 社区示例：核心域名、扩展服务、IP/ASN 兜底和
NTP 通用规则分为四份 classical rule set，再由无出口模版组合。界面标明社区来源、
非 Anthropic 官方且默认未启用。规则集页与分流模版页始终展示这份目录；查看内容
不修改 desired。只有用户编辑并保存到草稿、把规则集加入自建模版，或将其（含
Claude Code 模版）添加为某设备分流时，才把对应规则集与模版写入草稿。

设备的 `name` 是允许空格和 Unicode 的显示元数据，`id` 则是进入 mihomo selector
命名空间的稳定技术标识，仍限制为字母、数字、下划线和连字符。Web GUI 从显示名称自动
生成无冲突 ID，已有设备改名时保持 ID 不变；旧文档没有 `name` 时以 `id` 回退显示。
总览的设备流量与最近租约会按规范化 MAC 合并登记名称，并优先于 DHCP hostname，因而
客户端不提供 hostname 时也不会继续显示为未知设备。

`same_lan` 不产生 OpenSurge DHCP lease。Control API 会从 mihomo 当前连接收集与 gateway
同网段的源 IPv4，并在设备登记页用 macOS ARP cache 尽力补 MAC；总览流量 inventory
则合并 lease、applied 静态设备与当前观察源。证据必须分层显示为 DHCP 已验证、静态登记、
流量已观察或邻居已观察。ARP/流量只证明近期观察，不是 MAC 身份认证；未经过 Mac、已经
离线或经 IPv6 绕过的同 LAN 设备不会因此被自动发现。

`same_lan` 的设备主键仍是稳定 `id`，运行规则只需唯一固定 IPv4；MAC 可以为空，仅作为
身份观察和后续迁移信息。空 MAC 不生成 dnsmasq reservation。离开 `same_lan` 进入 DHCP
拓扑时，GUI 只按登记 IPv4 接受唯一且格式有效的当前邻居 MAC，并在写入前显示给用户确认；
全部设备原本已有 MAC 时不弹窗。仍没有 MAC 的设备保持在 declarative policy 中，但
mode-aware compiled bundle 必须排除其 device、selector、rule 与 reservation，并在设备页
持续显示“需要 MAC / 策略暂停”。返回 `same_lan` 或补全 MAC 后重新编译即可恢复，禁止删除
资料、伪造 MAC 或让新 DHCP lease holder 继承旧 `SRC-IP-CIDR` 策略。

same-LAN applied IPv4 没有当前流量时，只有“恰好一个不同 IPv4、相同规范化邻居 MAC、
`neighbor_observed=true` 且存在活跃连接”的观察项可以进入地址变化提示。GUI 不静默写入：
它先禁用旧 `SRC-IP-CIDR` 对应的路由方式和 applied selectors，再由用户确认一次保存与
安全重载。更新只替换设备 IPv4，必须保留稳定 ID、名称、Profile、规则、egress mode 与
selector 选择。无观察证据时 selector 可标成预设；同 MAC 多地址或目标 IPv4 已被其他
desired 设备占用时必须 fail closed，不提供猜测式更新。

旧文件省略 `egress_mode` 时解析为 `legacy_fallback`，继续保持“设备覆盖 → 全局规则 →
设备默认兜底 → terminal MATCH”。GUI 会显示兼容提示并要求用户明确迁移到跟随或独立，
不会静默改变现有流量。

`inherit_global` Profile 的 `default_policies` 仍保留供以后切换模式，但没有 dedicated/legacy
设备引用时不生成 selector，也不把这些未使用候选加入 imported target 校验。

一个示例配置见 `docs/user/device-policy.zh-CN.md` 和
`examples/device-policy.example.json`。设备 IPv4 必须唯一；在当前网关网段内的地址
不能是网段、广播或网关地址。网段由 `gateway.lan_ip` 与 `gateway.lan_prefix_len`
决定。不在当前网段的登记是 dormant 而不是非法：完整 desired policy 与 digest 保留，
但 compiled/applied bundle 会将它从运行态设备、dnsmasq reservation、Mihomo IPv4
selector/规则和 IPv6 MAC→InUser 身份映射中同时排除。`GET /api/v1/devices` 通过
`out_of_lan_devices` 告诉 GUI 标记它。

同一条“dormant 而不是非法”的规则也适用于 `device_policy.protected_ipv4`：不在当前
网段的受保护地址被忽略而不是报错。这是刻意的死锁避免：设备只能通过设备页删除或改
地址，而改配置本身又要通过同一套校验，所以异网段设备不能成为启动或保存的硬阻断。

same-Wi‑Fi DHCP 场景还必须将 router、recovery device、LAN proxy 等地址写入
`device_policy.protected_ipv4`；reservation 不得占用。启动前会对 reservation 做 ARP
冲突探测：观察到不同 MAC 是硬错误；无应答不等于地址必定空闲，因此第二 DHCP server
仍应由真实客户端的 OFFER/ACK server identifier 证据排除。

## 和 imported profile 的关系

Mac 本机 source-scoped 模式规则排在最前，但下游设备源地址不会命中。device override
规则在所有设备模式下都位于 imported/managed 全局规则之前。独立模式的
设备默认 selector 同样位于全局规则之前；跟随模式没有 default selector；只有旧版兼容
模式把默认兜底放在全局规则之后、最终 `MATCH` 之前。imported profile 的 `MATCH`
必须是 terminal；其后还有实质规则时渲染会失败。

imported profile 使用 YAML AST 收集 proxy/group/provider 名称。生成的 `device/` group 和
`open-surge-ruleset-` provider namespace 不能与 imported 内容冲突；default candidate、rule
candidate 与 action 的有效运行结果只能引用已有目标或显式内置目标。

配置来源变化导致旧出口消失时，`device.PolicyResolution` 根据最终合成配置的 target
inventory 和历史 selector 选择派生有效策略。默认出口的当前选择消失或候选全部消失时
回退 `inherit_global`；设备规则的固定/当前出口消失时跳过规则及其 `REJECT` 兜底；
仅未选中候选消失时过滤候选并保留有效选择。规则集、模版和直接条件规则共用编译分支，
不会删除共享规则内容，也不影响其他设备、DHCP reservation 或 IPv6 身份映射。

原始 Policy、canonical JSON 和 digest 保留。applied bundle 保存 resolution inventory 与
selection context，读取快照时按该上下文重编译，不能根据后续订阅重算或恢复失效 selector。
`CompiledDevice.egress_mode` 表示实际模式；回退时 `configured_egress_mode` 保留原模式，
`policy_adjustments` 为 GUI 提供 default/rule slot、缺失名称和实际处理结果，不能误报为
未保存的草稿差异。来源恢复后在下次启动/重载重新解析原策略。

历史选择优先来自运行中/准备态核心；已跳过的 selector 从 applied bundle 或准备态记录中
保留原选择，避免后续预览读取因 live group 缺席而重新启用它。停止态只读 pinned Mihomo 1.19.30 的 bbolt
`cache.db` / `selected` bucket，使用相同版本 bbolt 依赖和有界锁等待。禁止复制运行中
数据库、增加第二个 writer、清缓存或启动额外核心读取选择。临时校验目录必须保留原
cache 路径。首次使用且没有缓存时使用第一候选；缓存无法读取且没有可信选择时，受影响
selector 保守回退/跳过。节点离线或协议不支持不是名称失效，不改变现有 UDP 规则。

导入源快照逐字节保留，运行配置通过 YAML AST 合成，兼容 flow/block collection、带引号的
顶层键与 terminal `MATCH`。生成 YAML 可以统一缩进；不得恢复依赖原始文本行拼接的实现。
具体合成契约见 [profile overlay](mihomo-profile-overlay.md)。

mihomo 对不支持 UDP 的出口会继续向下匹配。设备 selector/default 因而默认在同条件后插入
`REJECT` fallback；只有 policy 显式写 `on_unsupported: "fallthrough"` 才保留向下匹配。

大型共享 domain/IP 列表使用 HTTP rule-provider；`mrs` 仅适用于 `domain` 和
`ipcidr` behavior。Claude Code 内置内容是定期人工更新的固定示例快照，不是远程规则订阅，
也不构成对第三方可用性的验证。

## Desired 与 applied

设备保存由 Helper 使用当前 imported inventory 和真实 `mihomo -t` 校验完整候选，
不能只检查 JSON 结构。`GET /api/v1/devices` 同时返回 desired/applied 设备及解析后内容
发生变化的设备 ID，供界面区分已应用、待应用、待更新与待移除。

The configured policy is desired state. One start compiles it exactly once into
an immutable bundle, validates the final mihomo configuration before forwarding
is enabled, and saves the bundle plus digest as runtime applied state. Running
`devices` and `device-policy-select` consume that applied snapshot; a changed
or invalid desired file is surfaced as drift/error rather than reinterpreting a
running gateway. A healthy running gateway may apply a saved desired policy
through `reload`: compile and validate the complete candidate in an isolated
temporary runtime, including real `mihomo -t`, then perform one full stop/start
with that same immutable config. Validation failure leaves the current gateway
untouched. Reload is interrupting and is not a zero-downtime hot swap.

## 验证与实现

编译逻辑见 `internal/device/`、`internal/mihomo/device_policy.go`；真实设备身份、独立出口、连接刷新和停止清理见 [每设备策略门槛](../validation/test-gates.md#每设备策略门槛)。拓扑中的 MAC 观察仅用于路由归属，不构成防伪造认证。
