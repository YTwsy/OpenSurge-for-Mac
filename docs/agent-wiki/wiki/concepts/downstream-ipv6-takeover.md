# 下游 IPv6 接管

这条能力解决的是下游 LAN 的 IPv6，而不是把物理客户端的源地址塞进 macOS 系统 TUN。

```text
客户端自动 SLAAC/RA 或旁路由手工 ULA/路由/DNS
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

Web GUI 将这两个设置组合在“下游 IPv6”卡片中，但不改变配置语义。三个拓扑在 TUN
开启时都提供 `off / auto / always` 与 AAAA 控件。共享 L2 还提供
`ipv6_shared_l2_ready` 前置条件确认；切换拓扑时必须清除旧确认，不能沿用。旁路由页面
另有 IPv4/IPv6 填写速查，动态显示所选 Mac 接口的 link-local 默认网关。卡片应优先
解释设备会获得的地址、默认路由和 DNS，再显示运行时探测细节，不使用 `RA Override`
作为用户概念。

## 支持边界

- 三个拓扑都要求 `transparent.mode: "tun"`。
- `isolated_lan` 自动发布 RA/SLAAC/RDNSS。
- `same_wifi_dhcp` 也自动发布 RA/SLAAC/RDNSS，但必须先关闭主路由 RA/DHCPv6，或用
  RA Guard 消除竞争默认路由，并显式确认 shared-L2 readiness。
- `same_lan` 不发布 RA，只接入手工 ULA、Mac link-local 默认网关与 DNS 且没有
  竞争 IPv6 默认路由的选定客户端。
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
- 自动模式客户端 RDNSS：下游接口的 link-local 地址（dnsmasq `[fe80::]` 替换）
- 手工旁路由默认网关：Mac 下游接口 link-local 地址（带接口 scope）
- 手工旁路由 DNS：Mac 下游接口 link-local 地址（与默认网关相同，带接口 scope）

共享 L2 Lab 曾尝试让手工客户端直接使用 `fdfe:dcba:9878::1` 作为 DNS，但 vmnet bridge
上的 ULA 邻居发现没有可靠完成，查询未到达 dnsmasq。不要把 ULA listener 当作旁路由
客户端填写契约；link-local 网关与 DNS 的组合已经通过 TCP、UDP/QUIC、设备身份和回滚
门槛。

自动模式的 RA 使用 Medium 默认路由器优先级。不要显式配置 `high`；共享 LAN 的正确性
来自消除竞争 RA，而不是用更高优先级压过主路由。

broker 从 BPF 保留 source MAC，listener 将它映射到内部
`device:<id>`。设备 selector 名称仍是 `device/<id>/...`。内部字符串不能带 `/`，
因为 Mihomo `IN-USER` 会把 `/` 解释为多个用户的分隔符。IPv6 地址与 MAC 冲突时
fail closed；这项身份适用于分流归属，不应被宣传为防 MAC spoofing 的安全认证。

`same_wifi_dhcp` 的按设备 `upstream_router` 是 IPv4-only 绕行。IPv6 开启时仍保留
该设备的 MAC→`device:<id>` 映射，但只用于最前置的
`IN-TYPE,TUN + IN-USER,REJECT`，不会
恢复它的 selector、普通设备规则或 IPv4 流量统计。其他设备 IPv6 保持原策略。设备
仍可能获得 SLAAC/RDNSS，因此 UI 写“IPv6 出站已阻止”；若主路由 RA 未消除，设备可
从 OpenSurge packet path 外直接走 IPv6，这不是 OpenSurge 能按设备拦截的链路。

## 生命周期与验收

自动模式启动按 Mihomo → broker → gateway alias → dnsmasq RA；停止按 dnsmasq →
lifetime-zero RA → alias → broker → Mihomo。`same_lan` 使用同一生命周期但不发送启动
或撤销 RA。runtime state 的 `ipv6_ra_effective` 区分这两条清理路径。任何中间失败都
进入统一 rollback，并保留无法完成清理时的 runtime state 供重试。

单元/无 root 集成门槛可以证明配置、真实 dnsmasq 语法、patched Mihomo 的
TCP/UDP gVisor 注入和 InUser 规则。只有实际通过
对应拓扑的 `make lab-test-ipv6-userspace`、`make lab-test-ipv6-same-wifi` 或
`make lab-test-ipv6-same-lan`，才能宣称 macOS BPF、双客户端配置、TCP、UDP/QUIC
carrier、按设备策略和停止清理在真实 host-network 路径中完成验收。自动模式还必须
从客户端观察到 Medium 默认路由；旁路由门槛必须证明没有 RA 配置。

来源：`../../sources/decisions/downstream-ipv6-takeover.md`。
