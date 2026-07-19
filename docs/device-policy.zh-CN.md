# 每设备策略覆盖

OpenSurge 只运行一个 mihomo 进程；不会为每台设备启动一份 mihomo 或复制完整 profile。
它会将已登记 MAC 固定到 IPv4 DHCP 租约，为每台设备生成独立 selector group，并用
mihomo 的 `SRC-IP-CIDR` 规则区分流量。

这是可选功能。在 gateway 配置中指定 JSON 文件：

```yaml
device_policy:
  file: "./devices.json"
```

空的 [starter 文件](../examples/device-policy.example.json) 合法，但不会启用任何设备策略。
路径相对于 gateway 配置文件解析。设备 IPv4 必须唯一、位于 gateway 的 `/24`，且不能
是网段地址、广播地址或 `gateway.lan_ip`。

在 `same_wifi_dhcp` 中，还必须声明路由器、恢复设备、LAN proxy 等绝不能被 reservation
占用的静态地址：

```yaml
device_policy:
  file: "./devices.json"
  protected_ipv4: "192.168.1.1,192.168.1.21,192.168.1.101"
```

reservation 可位于动态 DHCP 池内，`devices --format json` 会显式标记这一关系；但不得
占用 `protected_ipv4`。same-Wi‑Fi DHCP 启动会主动触发 ARP，并在观察到不同 MAC 已占用
该地址时拒绝启动。没有 ARP 应答不能证明地址空闲，所以仍须保留路由器 DHCP 隔离和恢复
证据。

## 模型

项目不内置儿童、影音、IoT 或第三方规则内容；规则和模板由操作者自己提供。JSON 中有：

- `devices`：稳定 `id`、可选显示名称 `name`、MAC、固定 IPv4、profile 与明确的
  `egress_mode`；
- `profiles`：默认 selector 候选项与设备覆盖规则；
- `templates`：可复用的 profile 默认值和规则片段；
- `rule_sets`：inline 或 HTTP mihomo rule-provider。

以下只是格式示例，`Proxy` 必须已存在于 managed 或 imported 的全局 mihomo profile：

```json
{
  "templates": [
    {"id": "baseline", "default_policies": ["DIRECT", "Proxy"]}
  ],
  "rule_sets": [
    {"id": "media", "behavior": "domain", "payload": ["media.example"]}
  ],
  "profiles": [
    {
      "id": "phone",
      "template": "baseline",
      "rules": [
        {"id": "block-udp", "match": {"protocols": ["udp"]}, "action": "REJECT"},
        {"id": "media", "match": {"rule_sets": ["media"]}, "policies": ["Proxy", "DIRECT"]}
      ]
    }
  ],
  "devices": [
    {"id": "alice-phone", "name": "Alice Phone", "mac": "aa:bb:cc:dd:ee:01", "ipv4": "192.168.50.101", "profile": "phone", "egress_mode": "dedicated"}
  ]
}
```

`name` 只是显示元数据，可以包含空格和 Unicode 字符；稳定 `id` 仍只允许字母、数字、
下划线和连字符，因为它会进入 `device/<device-id>/default` 等 selector 名称。Web GUI
让用户填写显示名称并自动生成无冲突的内部 ID；以后修改名称不会改变现有 ID。旧文件没有
`name` 时继续把 `id` 当作显示名称。

`egress_mode` 有两种明确取值：

- `inherit_global`：设备专属规则仍优先；未命中流量继续走与本机相同的全局规则和
  terminal `MATCH`；
- `dedicated`：未命中的公网流量会在全局规则前进入设备自己的
  `device/<device-id>/default` selector。局域网、私网、link-local、CGNAT 和 multicast
  目标仍保持 `DIRECT`。

Web GUI 新登记设备默认使用 `inherit_global`。旧文件没有 `egress_mode` 时不会被静默改变，
而会以 `legacy_fallback` 保留原来的“全局规则优先、设备出口兜底”语义；GUI 会明确提示
用户选择一种新模式。

只有跟随设备使用的 Profile 仍会保留 `default_policies` 作为以后切换独立模式的配置，
但这些未渲染的候选不会参与当前 imported profile 引用校验；真正生成独立或兼容 selector
时才校验。

在 `dedicated`（以及旧版兼容）模式中，`default_policies` 会生成
`device/<device-id>/default`。规则含 `policies` 时会生成
`device/<device-id>/<rule-id>` 的独立 selector；规则含 `action` 时直接指向
`DIRECT`、`REJECT` 或已有全局 mihomo group。

启动前会校验候选项和 action 是否引用 imported profile 中存在的 proxy/group；内置目标
仅显式允许 `DIRECT`、`REJECT`、`REJECT-DROP`、`REJECT-TINYGIF`。`device/` 是生成
group 的保留命名空间，`open-surge-ruleset-` 是生成 provider 的保留命名空间，imported
profile 不能占用它们。

## 匹配与顺序

`domains`、`ip_cidrs`、`protocols`（`tcp`/`udp`）、`ports` 与 `rule_sets` 可以组合。
不同字段是 AND，同一字段里的多个值是 OR，会编译为多条 mihomo 规则：

```text
AND,((SRC-IP-CIDR,192.168.50.101/32),(DOMAIN-SUFFIX,media.example),(NETWORK,tcp)),device/alice-phone/media
```

设备专属覆盖在所有模式下都先于全局规则。`inherit_global` 随后继续进入全局规则与
terminal `MATCH`；`dedicated` 的顺序是按设备源地址限定的本地/私网 `DIRECT` 保护 →
设备专属覆盖 → 设备默认 selector → imported/managed 全局规则 → terminal `MATCH`。
缺少模式的旧文件仍保持设备默认 selector 位于全局规则之后、`MATCH` 之前。imported
profile 的 `MATCH` 必须位于最后；若其后还有规则，OpenSurge 会拒绝渲染。

## 不支持 UDP 的出口

mihomo 遇到不支持 UDP 的出口会继续向下匹配。因而设备 selector/default 默认采用
fail-closed：每条 selector/default 规则后都会紧跟同条件的 `REJECT`，避免 QUIC 或其他
UDP 流量继续落入后续全局规则或 `MATCH,DIRECT`。

可在 template、profile 或单条 rule 写入 `on_unsupported: "fallthrough"`，但只能在明确
希望后续规则承担 fallback 时使用；默认是 `"reject"`。group 名存在不等于其节点支持
UDP，provider 候选仍须以真实流量验证。

## 大型规则集和模板

`rule_sets` 支持 `inline`/`http`，以及 `domain`、`ipcidr`、`classical` behavior。HTTP
provider 可用 `yaml`、`text`、`mrs`；MRS 只适用于 `domain` 和 `ipcidr`。大型共享域名/IP
列表应使用 HTTP MRS；模板只复用策略选择和规则片段，不复制完整 mihomo profile。

## 操作与验证

```sh
./bin/omg devices --config ./config.yaml --format json

./bin/omg device-policy-select \
  --config ./config.yaml \
  --device alice-phone \
  --slot default \
  --policy Proxy
```

`device-policy-select` 只改变指定设备的 selector，不会改变其他设备或全局策略组。只有
已经 applied 的 `dedicated` 或旧版兼容设备才有 `default` slot；`inherit_global` 设备明确
不生成默认 selector。

网关已经健康运行时，可用安全重载应用保存后的 desired 配置：

```sh
sudo ./bin/omg reload --config ./config.yaml
sudo ./bin/omg reload --config ./config.yaml --format json
```

`reload` 先在隔离的临时 runtime 中渲染完整候选，检查接口、protected/reservation IPv4
冲突，并执行真实 `mihomo -t`。预校验失败不会停止现有网关；全部通过后才执行正常的完整
stop/start。这是会短暂中断连接的重载，不是服务级热替换或零中断承诺。

## desired 与 applied

`start` 只编译一次 policy bundle，以同一不可变 bundle 渲染 DHCP 与 mihomo，并在启用
forwarding 前执行 mihomo 校验。成功启动会写入
`runtime/device-policy.applied.json`，并将其 digest 写入 `runtime/state.json`。gateway
运行时，`devices` 和 `device-policy-select` 都读取 applied snapshot；`devices` 会比较当前
desired digest 并返回 `drift`。desired 文件即使暂时无效，也会以 `desired_error` 返回，而
不会遮住仍在运行的 applied policy。

编辑 `devices.json` 不会自动 reload gateway。网关健康运行时使用 `reload`；网关停止时，
desired 配置会在下次正常 `start` 应用。启动时会清除受管 MAC 仍指向旧 reservation IPv4
的 stale lease row，随后应等待新的 DHCP ACK 再测试流量。`lease_active` 只表示租约未
过期，不表示设备可达；只有 applied reservation 与
lease 的 MAC、IPv4、expiry 全部匹配时，`policy_identity_ready` 才为 true。

Web GUI 将两类操作持续分开：绿色表示 applied selector 的“即时生效”，黄色表示设备
身份、路由方式、selector 成员和规则的“保存后重载”。只有 applied 的独立出口设备显示
可即时切换的默认 selector；跟随设备明确显示它正在使用全局规则。普通登记默认创建
`<device-id>-policy` 私有 Profile；设备首次修改共享 Profile 或继承 Template 的 Profile
时，GUI 会把解析后的有效配置复制为不再继承 Template 的私有 Profile，只更新这台设备
的引用。

`same_lan` 手工网关不运行 OpenSurge DHCP。该模式下，设备页从 mihomo 当前连接中提取与
`gateway.lan_ip` 同 `/24` 的源 IPv4，并用 macOS ARP 邻居表尽力补充 MAC，列入“当前经过
Mac 的设备”供登记。总览设备流量会合并 DHCP lease、applied 静态设备和当前观察到的
same-LAN 源 IPv4：已登记静态 IPv4 可以获得名称、连接、速率、累计流量与出口归属，未登记
但正在经过 Mac 的 IPv4 也以临时设备显示。ARP 与流量观察不是 DHCP 身份验证；MAC 未解析
时仍需用户手工填写，且静态设备必须在主路由侧保持稳定 IPv4。

当前设备策略仍以 IPv4 `SRC-IP-CIDR` 执行。DHCP 模式提供 MAC 绑定租约的精确身份证据；
`same_lan` 只提供静态登记、当前流量与可选 ARP 邻居观察，不把这些证据冒充 DHCP 验证。
尚未提供 IPv6 设备身份、mihomo 内 MAC 匹配或预置第三方规则内容。

数据面 gate：

```sh
make lab-up
sudo -v
make lab-test-tun-device-policy
make lab-down
```

它会验证两个 Lima 客户端的 `.101`/`.102` 固定租约：先证明独立出口设备会在全局
`MATCH` 前使用 selector，并证明跟随设备没有默认 selector 且走全局 `MATCH`；随后把
跟随设备重载为独立模式，验证互不影响的 selector 切换与设备级 IP `REJECT`。脚本会制造 desired drift，调用真实
`omg reload`，要求网关继续运行且 desired/applied digest 收敛，再复查 selector 隔离与新
规则。同时验证精确 DHCP identity，以及 HTTP-only 出口上的 UDP/443 必须记录为 `REJECT`
而不能 fall through 到 `DIRECT`。规则、模板与 provider 的编译由单元测试覆盖，不需要为
每条操作者规则运行 Lab。
