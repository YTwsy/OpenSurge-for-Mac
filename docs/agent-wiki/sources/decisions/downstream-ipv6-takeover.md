---
title: Downstream IPv6 takeover
kind: decision
status: implemented-validated
---

# 下游 IPv6 接管决策

OpenSurge 的下游 IPv6 接管只在 `gateway.mode: "isolated_lan"` 中启用。旁路由和
同网段 DHCP 模式仍有上游路由器 RA；在没有 RA suppression / RA Guard 之前启用会
形成绕过 Mac 的第二条 IPv6 默认路由，因此配置必须 fail closed。

用户控制面保持两个独立设置：

- `dns.ipv6` 控制 mihomo DNS 是否回答 AAAA，并在开启时使用
  `fdfe:dcba:9876::/64` fake IPv6；
- `transparent.tun_ipv6` 取 `off | auto | always`，控制是否为独立下游 LAN
  发布 IPv6 网关、DNS 和用户态透明数据面。`auto` 只在上游接口同时有公网全局
  IPv6 地址（排除 ULA）和 IPv6 默认路由时生效；`always` 强制建立下游路径。

下游 IPv6 不从物理接口路由进 macOS 系统 utun。root broker 在下游 Ethernet 的
BPF 上读取入站 IPv6 Ethernet frame，把 L3 packet 和 source MAC 通过权限为 0600
的 Unix datagram 交给打补丁的 Mihomo `opensurge-packet` listener。listener 用
sing-tun gVisor 处理 TCP/UDP，把 MAC 映射为内部 `device:<id>` InUser，再复用既有
设备规则和 outbound。返回包沿 Unix datagram 回到 broker，并按已观察的
IPv6-to-MAC 邻居表写回 Ethernet。QUIC 由 UDP/443 路径承载；这个实现不宣称代理
ICMPv6、ESP 或任意非 TCP/UDP 协议。

内部身份不能使用设备策略组的 `device/<id>` 字符串，因为 Mihomo 的 `IN-USER`
规则把 `/` 当成多个用户名的分隔符。策略组继续使用用户可见的
`device/<id>/...`，packet listener 和对应规则使用单值 `device:<id>`。

dnsmasq 在下游发布 `fdfe:dcba:9878::/64` 的 SLAAC/RA 和默认路由，并把
下游接口的 link-local 地址发布为 RDNSS。`fdfe:dcba:9878::1` 保留为 Mac 的
下游 gateway alias 和 dnsmasq 监听地址；它不作为客户端收到的 RDNSS。
Mihomo 系统 TUN 地址使用独立的
`fdfe:dcba:9877::1/126`。三段前缀必须保持互不重叠，fake IPv6 不能被本地 ULA
直连规则吞掉。

启动顺序是 Mihomo listener、BPF broker、下游 IPv6 gateway alias、dnsmasq RA；
停止顺序是 dnsmasq、router/prefix lifetime-zero withdrawal、alias removal、broker、
Mihomo。broker PID/fingerprint 必须在修改接口前写入 runtime state。IPv6 source
地址被第二个 MAC 使用时保持第一次映射并拒绝冲突，不静默改绑。

`always` 不等于凭空获得公网 IPv6。上游没有原生 IPv6 时，fake IPv6 目标仍可由
支持相应域名/UDP 的代理出口承载；`DIRECT` 到真实公网 IPv6 地址仍会因为没有上游
IPv6 route 而失败。HTTP-only 代理也不能承载 UDP/QUIC。

事实来源：

- `internal/ipv6packet/`
- `patches/mihomo/overlay/listener/opensurge_packet/`
- `internal/macosipv6/`
- `internal/dhcp/template.go`
- `internal/gateway/manager.go`
- `internal/mihomo/device_policy.go`
- `tests/lab/lab.sh`
- dnsmasq `--dhcp-option=option6:dns-server,[fe80::]` link-local substitution:
  <https://thekelleys.org.uk/dnsmasq/docs/dnsmasq-man.html>
