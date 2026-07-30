---
title: IPv6 DNS and TUN modes
kind: decision
status: active
---

# IPv6 DNS 与 TUN 模式

OpenSurge 把 IPv6 拆成两个独立配置：

- `dns.ipv6` 决定 mihomo DNS 是否回答 AAAA 查询；开启时使用
  `fdfe:dcba:9876::/64` 作为 fake IPv6 范围。
- `transparent.tun_ipv6` 决定 mihomo TUN 是否配置 IPv6，可选 `off`、`auto`
  和 `always`。

`auto` 只有在上游接口同时存在非 link-local IPv6 地址和 IPv6 默认路由时才实际启用。
`always` 会向 mihomo 设置 `SKIP_SYSTEM_IPV6_CHECK=1`，用于明确需要强制启用的网络和
Virtual Lab。运行状态必须同时保存 requested mode 和 effective bool，不能把
`auto` 的期望值冒充成已生效状态。

这两个设置只覆盖 DNS 语义和宿主 Mac 上的 mihomo VIF：

- `dns.ipv6` 不建立 IPv6 数据面；
- `transparent.tun_ipv6` 不改变 `net.inet6.ip6.forwarding`；
- dnsmasq 不发送 Router Advertisement，也不分配下游 ULA；
- OpenSurge 不宣称这两个设置接管了下游设备的 IPv6。

Virtual Lab 已证明本机发起、路由到 mihomo VIF fake IPv6 的 HTTPS 可以经 TUN
和受控出口完成。同时也证明，把下游客户端的 IPv6 经 macOS 内核 forwarding 送向
mihomo utun 并不可行：数据包到达下游 bridge，但没有进入 utun，Darwin IPv6 统计将其
记为不可转发。RA、静态路由、PF `route-to` 和 NAT66 都没有改变这个结果。

因此，下游 IPv6 接管必须作为独立能力重新设计，采用经证明的用户态数据面或其他
macOS 支持路径，并拥有单独的真实设备门槛。在此之前，所有拓扑都不广播 OpenSurge
RA，客户端可能沿原路由器 IPv6 绕过 Mac。
