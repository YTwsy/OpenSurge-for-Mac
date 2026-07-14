# 每设备策略覆盖

当任务涉及设备身份、每设备默认出口、设备规则覆盖，或如何把设备策略安全叠加到
mihomo profile 时，先读此页。

OpenSurge 只运行一个 mihomo。可选的 `device_policy.file` JSON 文件为每台设备记录
MAC、固定 IPv4 与 profile；编译时将它们转换为 DHCP reservation、独立 selector
group，以及以 `SRC-IP-CIDR` 区分来源的 mihomo 规则。它不是“一台设备一份完整
mihomo YAML”。

## 策略模型

- 每台设备总有 `device/<id>/default` selector。
- 含 `policies` 的设备规则会获得 `device/<id>/<rule-id>` selector；
  `device-policy-select` 只能选择此设备拥有的 selector。
- 含 `action` 的规则直接发往 `DIRECT`、`REJECT` 或已有全局 mihomo group。
- `domains`、`ip_cidrs`、`protocols`、`ports` 和 `rule_sets` 可组合；字段之间为
  AND，同字段多个值为 OR。
- `templates` 只复用默认候选与规则片段；项目不预置儿童、影音或第三方规则内容。

Web GUI 的设备主路径不要求用户先理解这些复用对象：登记默认创建
`<device-id>-policy` 私有 Profile。若设备仍引用共享 Profile 或继承 Template，第一次从
设备规则区修改候选或规则时，会将解析后的有效内容复制为无 Template 的确定性私有
Profile，只改变该设备引用；ID 冲突时追加数字后缀。`PolicySet` schema 保持不变。

一个示例配置见 `docs/device-policy.zh-CN.md` 和
`examples/device-policy.example.json`。设备 IPv4 必须唯一、在 gateway `/24` 内，
且不能是网段、广播或网关地址。

same-Wi‑Fi DHCP 场景还必须将 router、recovery device、LAN proxy 等地址写入
`device_policy.protected_ipv4`；reservation 不得占用。启动前会对 reservation 做 ARP
冲突探测：观察到不同 MAC 是硬错误；无应答不等于地址必定空闲，因此第二 DHCP server
仍应由真实客户端的 OFFER/ACK server identifier 证据排除。

## 和 imported profile 的关系

device override 规则在 imported/managed 全局规则之前，设备默认规则在全局规则之后，
最终 `MATCH` 之前。imported profile 的 `MATCH` 必须是 terminal；其后还有实质规则
时，渲染会失败，以免设备默认出口被无声吞掉。

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
`ipcidr` behavior。此处只是配置编译能力，并不代表内置或验证了任何第三方规则集。

## 验证边界

`make test` 覆盖 JSON 校验、模板合并、domain/IP/protocol 组合、rule-provider 和
imported profile 的排序。

`make lab-test-tun-device-policy` 是数据面门槛：它使用两个 Lima VM，验证 `.101` 与
`.102` 的固定租约、两台设备不同的 TUN 出口、互不影响的 selector 切换，以及设备级
域名 `REJECT`。它还制造 desired drift 并调用真实 `omg reload`，验证网关继续运行、
applied snapshot/state digest 与 desired 收敛、精确 lease identity 仍成立，随后复查两台
设备 selector 仍相互隔离；同时验证 HTTP-only selector 选中时 UDP/443 的 fail-closed
`REJECT`。它不需要、也不会为
操作者写的每条 domain/protocol/template 规则重复运行 Lab。

当前只支持 MAC 绑定 IPv4 DHCP 租约和 IPv4 `SRC-IP-CIDR` 身份；未提供 IPv6 设备
身份或 mihomo 内 MAC 匹配。
