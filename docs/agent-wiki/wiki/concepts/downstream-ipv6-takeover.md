# 下游 IPv6 接管

这条能力解决的是独立下游 LAN 的 IPv6，而不是把物理客户端的源地址塞进 macOS
系统 TUN。

```text
客户端 SLAAC + 默认路由 + RDNSS
              │
              ▼
       下游 Ethernet / bridge
              │ IPv6 frame + source MAC
              ▼
       macOS BPF packet broker
              │ 0600 Unix datagram
              ▼
 patched Mihomo opensurge-packet listener
              │ gVisor TCP / UDP
              ▼
 IN-USER(device:<id>) → 设备规则 → outbound
```

## 两个设置

`dns.ipv6` 只决定 DNS 是否回答 AAAA 和生成 fake IPv6。它不会单独发布 RA 或建立
下游透明路由。

`transparent.tun_ipv6` 决定下游接管：

- `off`：不发布 OpenSurge IPv6；
- `auto`：上游接口有公网全局 IPv6 地址（排除 ULA）且存在该接口的 IPv6 默认路由时启用；
- `always`：无论上游是否有原生 IPv6都建立下游 RA 和用户态 packet path。

这两个开关可以独立设置。常规全屋 IPv6 应同时开启；只开启接管时，IPv6 字面地址
仍可进入路径，但普通域名不会从 OpenSurge DNS 获得 AAAA。

## 支持边界

- 当前只允许 `gateway.mode: "isolated_lan"` 和
  `transparent.mode: "tun"`。
- 同 LAN / 同 Wi-Fi 模式在实现 RA suppression 或 RA Guard 前必须拒绝开启，避免
  客户端继续使用主路由器 RA 绕过 Mac。
- 数据面覆盖 TCP 和 UDP；QUIC 作为 UDP/443 覆盖。不要宣称任意 IPv6 协议均可代理。
- 下游 IPv6 ingress 不走系统 utun；现有 IPv4 下游透明代理和 Mac 本机透明代理仍
  使用受支持的 Mihomo TUN 主线。
- `always` 在无原生 IPv6 上游时仍依赖能承载目标流量的代理。真实 IPv6 `DIRECT`
  不会因此获得出口，HTTP-only 代理也不能承载 UDP/QUIC。

## 地址和身份

- fake IPv6：`fdfe:dcba:9876::/64`
- Mihomo 系统 TUN：`fdfe:dcba:9877::1/126`
- 下游 SLAAC：`fdfe:dcba:9878::/64`
- 下游 gateway alias / DNS listener：`fdfe:dcba:9878::1`
- 客户端 RDNSS：下游接口的 link-local 地址（dnsmasq `[fe80::]` 替换）

broker 从 BPF 保留 source MAC，listener 将它映射到内部
`device:<id>`。设备 selector 名称仍是 `device/<id>/...`。内部字符串不能带 `/`，
因为 Mihomo `IN-USER` 会把 `/` 解释为多个用户的分隔符。IPv6 地址与 MAC 冲突时
fail closed；这项身份适用于分流归属，不应被宣传为防 MAC spoofing 的安全认证。

## 生命周期与验收

启动按 Mihomo → broker → gateway alias → dnsmasq RA；停止按 dnsmasq → lifetime-zero
RA → alias → broker → Mihomo。任何中间失败都进入统一 rollback，并保留无法完成清理
时的 runtime state 供重试。

单元/无 root 集成门槛可以证明配置、真实 dnsmasq 语法、patched Mihomo 的
TCP/UDP gVisor 注入和 InUser 规则。只有实际通过
`make lab-test-ipv6-userspace`，才能宣称 macOS BPF、双客户端 SLAAC、TCP、UDP/QUIC
carrier、按设备策略和停止撤销在真实 host-network 路径中完成验收。

来源：`../../sources/decisions/downstream-ipv6-takeover.md`。
