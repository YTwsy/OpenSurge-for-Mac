# 每设备策略覆盖

当任务涉及设备身份、每设备默认出口、设备规则覆盖，或如何把设备策略安全叠加到
mihomo profile 时，先读此页。

OpenSurge 只运行一个 mihomo。可选的 `device_policy.file` JSON 文件为每台设备记录
固定 IPv4、可选 MAC 与 profile；编译时按拓扑将它们转换为 DHCP reservation、独立 selector
group，以及以 `SRC-IP-CIDR` 区分来源的 mihomo 规则。它不是“一台设备一份完整
mihomo YAML”。

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

Web GUI 的设备主路径只暴露三个对象：规则集、分流模版和设备分流。默认展开的
“规则库”用三个 tab 编辑它们，旧的独立设备规则卡片不再渲染。设备卡保持原有出口交互，
“编辑设备分流”会聚焦规则库的对应设备。登记仍创建 `<device-id>-policy` 私有 Profile，
但 Profile 不作为规则库中的用户对象。路由模式修改属于 save-and-reload；只有 applied
的独立出口设备显示可即时切换的 default selector。首次修改共享或旧式继承 Profile 时，
前端仍把有效内容私有化到该设备，避免修改其他设备。

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

一个示例配置见 `docs/device-policy.zh-CN.md` 和
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
candidate 与 action 也必须引用已有目标或显式内置目标。

导入 section 的原始 YAML 文本会保留。追加生成的 selector、rule-provider 与规则时，必须
沿用该 section 已有顶层 item 的缩进；订阅常见的 4 空格缩进不能与 OpenSurge 默认的 2
空格混用。识别 terminal `MATCH` 时也必须同时接受带单引号、双引号和未加引号的规则，
以确保设备 default 规则仍插在全局 `MATCH` 之前。

mihomo 对不支持 UDP 的出口会继续向下匹配。设备 selector/default 因而默认在同条件后插入
`REJECT` fallback；只有 policy 显式写 `on_unsupported: "fallthrough"` 才保留向下匹配。

大型共享 domain/IP 列表使用 HTTP rule-provider；`mrs` 仅适用于 `domain` 和
`ipcidr` behavior。Claude Code 内置内容是定期人工更新的固定示例快照，不是远程规则订阅，
也不构成对第三方可用性的验证。

## 验证边界

`make test` 覆盖 JSON 校验、模板合并、domain/IP/protocol 组合、rule-provider 和
imported profile 的排序。

`make lab-test-tun-device-policy` 是数据面门槛：它使用两个 Lima VM，验证 `.101` 与
`.102` 的固定租约、独立出口优先于全局 `MATCH`、跟随设备不生成 default selector 且
走全局 `MATCH`；再把跟随设备重载成独立模式，验证两台设备不同的 TUN 出口、互不影响
的 selector 切换，以及设备级 IP `REJECT`。它还制造 desired drift 并调用真实 `omg reload`，验证网关继续运行、
applied snapshot/state digest 与 desired 收敛、精确 lease identity 仍成立，随后复查两台
设备 selector 仍相互隔离；同时验证 HTTP-only selector 选中时 UDP/443 的 fail-closed
`REJECT`。它不需要、也不会为
操作者写的每条 domain/protocol/template 规则重复运行 Lab。

系统 TUN 路径仍只用 IPv4 `SRC-IP-CIDR`。IPv6 packet path 在三种拓扑中把观察到的
source MAC 映射成 patched Mihomo 的 `IN-USER(device:<id>)`；普通设备复用设备策略，
`upstream_router` 设备只命中最优先的全 IPv6 `REJECT`。DHCP 模式提供 MAC 绑定租约
证据；`same_lan` 提供静态配置与近期流量/邻居观察，但不冒充 DHCP identity。这里的
MAC 是路由身份，不是防伪造认证。
