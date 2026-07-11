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

- `devices`：`id`、MAC、固定 IPv4 和 profile；
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
    {"id": "alice-phone", "mac": "aa:bb:cc:dd:ee:01", "ipv4": "192.168.50.101", "profile": "phone"}
  ]
}
```

`default_policies` 会生成 `device/<device-id>/default`。规则含 `policies` 时会生成
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

生成顺序固定为：设备专属覆盖 → imported/managed 全局规则 → 设备默认 selector →
全局终结 `MATCH`。imported profile 的 `MATCH` 必须位于最后；若其后还有规则，OpenSurge
会拒绝渲染，避免设备默认策略被悄悄吞掉。

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

`device-policy-select` 只改变指定设备的 selector，不会改变其他设备或全局策略组。

## desired 与 applied

`start` 只编译一次 policy bundle，以同一不可变 bundle 渲染 DHCP 与 mihomo，并在启用
forwarding 前执行 mihomo 校验。成功启动会写入
`runtime/device-policy.applied.json`，并将其 digest 写入 `runtime/state.json`。gateway
运行时，`devices` 和 `device-policy-select` 都读取 applied snapshot；`devices` 会比较当前
desired digest 并返回 `drift`。desired 文件即使暂时无效，也会以 `desired_error` 返回，而
不会遮住仍在运行的 applied policy。

编辑 `devices.json` 不会 reload gateway；MVP 约定是 `stop` 后编辑、再 `start`。启动时会
清除受管 MAC 仍指向旧 reservation IPv4 的 stale lease row，随后应等待新的 DHCP ACK 再
测试流量。`lease_active` 只表示租约未过期，不表示设备可达；只有 applied reservation 与
lease 的 MAC、IPv4、expiry 全部匹配时，`policy_identity_ready` 才为 true。

当前设备身份仅覆盖 MAC 绑定的 IPv4 DHCP 租约和 IPv4 `SRC-IP-CIDR`；尚未提供 IPv6
设备身份、mihomo 内 MAC 匹配或预置第三方规则内容。

数据面 gate：

```sh
make lab-up
sudo -v
make lab-test-tun-device-policy
make lab-down
```

它会验证两个 Lima 客户端的 `.101`/`.102` 固定租约、独立 TUN policy/egress、互不影响
的 selector 切换，以及设备级域名 `REJECT`。同时验证 applied bundle/state digest、精确
DHCP identity、policy 文件编辑后的 desired/applied drift，以及 HTTP-only 出口上的 UDP/443
必须记录为 `REJECT` 而不能 fall through 到 `DIRECT`。规则、模板与 provider 的编译由单元
测试覆盖，不需要为每条操作者规则运行 Lab。
