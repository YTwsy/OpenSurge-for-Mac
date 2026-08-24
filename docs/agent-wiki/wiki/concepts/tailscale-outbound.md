# Tailscale 出站

OpenSurge 把 mihomo `type: tailscale` 实现为一个受管 outbound，而不是第二个
系统 VPN 开关。mihomo 内部的 tsnet 负责 control plane、WireGuard、DERP、
NAT traversal 和 Tailnet DNS；OpenSurge 负责身份生命周期、目标与来源授权、
路由冲突检查、配置编译和 GUI。

## 稳定名称与身份

托管代理内部名称固定为 `open-surge/tailscale`。这是 device policy 和
mihomo selector 使用的稳定编译目标；GUI 应显示用户的 `display_name`，不应
把内部名称当成产品文案。导入 profile 不得占用这个保留名称。

Auth Key 是 write-only secret，保存在配置目录下权限为 `0600` 的独立文件。
`config.yaml` 只记录该文件路径，Control API 只返回是否已有 key。第一次
注册后，mihomo 的 `state-dir` 由 OpenSurge 持久化；重启、reload 和暂时停用
仍使用同一本地节点身份。

停用不删除 key 或 state。“忘记本地身份”必须满足 Tailscale 已停用、
网关已停止且 state 路径确实是 OpenSurge 托管路径；它只删除本地
state，不声称已从 Tailscale / Headscale 后台注销设备。

## 编译顺序与授权

Tailnet 规则在普通 CGNAT/RFC1918 `DIRECT` 保护规则之前，但只包含用户明确
配置的目标：

- MagicDNS domain suffix；
- peer IP/CIDR；其中 `100.64.0.0/10` 和 Tailscale IPv6 ULA 空间只允许
  `/32` 或 `/128` 的精确 peer，拒绝整段接管；
- 已批准的远端 subnet route。

每条目标规则还必须同时命中被授权的来源：Mac 本机、所有有效登记设备，
或明确的设备 ID 集合。未授权流量不会因为它使用 `100.64.0.0/10` 或
RFC1918 地址就被 Tailscale 接管。命中后直接选择 Tailscale outbound；节点
离线时 fail closed，不增加 `DIRECT` 回退。

TUN `route-address` 只增加 peer CIDR 和 subnet route 这些明确的网段，以覆盖
原有私网 route exclusion。不删除对整个 RFC1918 和 CGNAT 空间的保护。
远端 subnet route 必须是 private IPv4 或 ULA，必须启用 `accept-routes`，并且
不得与 OpenSurge 当前 LAN 重叠。

## Tailnet 与 Exit Node 角色

Tailnet-only outbound 只为上述明确目标服务，不得成为 device policy 的
公网出口候选。它也不使用普通公网 `generate_204` 做健康检查；健康状态
显示为 `available_on_demand`。mihomo 只在 outbound 首次收到请求时启动
Tailscale 节点；OpenSurge 在网关启动、重载和 mihomo 恢复后发送一次最佳努力
预热请求，把大部分 lazy-start 成本提前，但产品文案仍须说明首次业务访问可能重试。

配置明确的 `exit-node` 后，同一 outbound 可以作为公网出口，才会被
device policy 校验器和 GUI 加入候选。Exit Node 不覆盖全局网关模式；它是
设备 selector 中的普通成员。Exit Node 使用公网延迟探测，但离线时不
自动回退到 `DIRECT`。

## 当前能力边界

这个集成不运行独立 `tailscaled`，也不与 Tailscale App 共享本地 state。
它是 outbound-only：不发布 OpenSurge LAN route，不让 OpenSurge 成为 subnet router，
不通过该节点提供入站服务。Headscale 使用同一实现，只替换
`control-url`。

## 验证边界

单元测试覆盖配置 round-trip、密钥不回显、路由冲突、规则顺序、imported/
managed 编译与 rollback。使用项目补丁构建的 mihomo 运行 `-t` 只能证明
最终 YAML 被当前内核接受。真实 TUN、DNS、UDP/QUIC、subnet router 与
Exit Node 路径必须等待专门 Virtual Lab 或真机门槛，不能从单元测试外推。
