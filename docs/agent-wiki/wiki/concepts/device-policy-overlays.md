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

一个示例配置见 `docs/device-policy.zh-CN.md` 和
`examples/device-policy.example.json`。设备 IPv4 必须唯一、在 gateway `/24` 内，
且不能是网段、广播或网关地址。

## 和 imported profile 的关系

device override 规则在 imported/managed 全局规则之前，设备默认规则在全局规则之后，
最终 `MATCH` 之前。imported profile 的 `MATCH` 必须是 terminal；其后还有实质规则
时，渲染会失败，以免设备默认出口被无声吞掉。

大型共享 domain/IP 列表使用 HTTP rule-provider；`mrs` 仅适用于 `domain` 和
`ipcidr` behavior。此处只是配置编译能力，并不代表内置或验证了任何第三方规则集。

## 验证边界

`make test` 覆盖 JSON 校验、模板合并、domain/IP/protocol 组合、rule-provider 和
imported profile 的排序。

`make lab-test-tun-device-policy` 是数据面门槛：它使用两个 Lima VM，验证 `.101` 与
`.102` 的固定租约、两台设备不同的 TUN 出口、互不影响的 selector 切换，以及设备级
域名 `REJECT`。它不需要、也不会为操作者写的每条 domain/protocol/template 规则重复
运行 Lab。

当前只支持 MAC 绑定 IPv4 DHCP 租约和 IPv4 `SRC-IP-CIDR` 身份；未提供 IPv6 设备
身份或 mihomo 内 MAC 匹配。
