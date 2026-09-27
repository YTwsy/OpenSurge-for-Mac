# Mac 系统 DNS 与 IPv6 TUN

`local_system_dns.enabled` 默认开启，仅在 TUN 与自动路由同时开启时生效。
它把配置的 upstream interface 对应的 macOS 网络服务 DNS 设为
`114.114.114.114`。这个地址是进入 TUN DNS 劫持的目的地址，mihomo 的实际
`nameserver` / `proxy-server-nameserver` 等解析器仍来自受管或导入配置。
UDP 与 TCP 53 都必须被劫持。LAN 路由器和链路本地 DNS 不保证进入 TUN。

## 生命周期与所有权

启动在改变主机前读取网络服务 UUID、接口、自动/手动 DNS 与原服务器列表，
读取失败必须终止，不能当成“自动”。快照写入 runtime state 后，等 mihomo
TUN 与路由检查通过、DHCP/DNS 和 pf 就绪，再持久化 DNS 写入意图并应用设置。
应用后读回网络服务 DNS。`verified_at` 是启动证据，不代表持续监控。

停止和回滚在关闭 DNS/TUN 前恢复原设置。`restart-mihomo` 在关闭旧引擎前
恢复 DNS，新引擎就绪后再接管；失败保留恢复快照。跨 boot 的 interrupted
清理也恢复 DNS，但不向旧 PID 发信号、不改新 boot 的 pf/forwarding。
恢复失败保留状态和仍运行的服务，允许再次 Stop 重试。

恢复按 UUID 和接口确认服务，允许服务改名；服务被删除或移到其他接口则报错。
当前 DNS 不等于 OpenSurge 设置或原值时，放弃所有权并保留外部修改。
这不能识别其他软件恰好写入相同值的情况，也不是引擎崩溃后的网络 kill switch。

只修改该服务的 DNS 服务器，不改搜索域、其他服务、系统 supplemental resolver、
`/etc/resolver` 或 mDNS。原解析器只用于 `.lan`、`home.arpa`、私有反向域和该服务
搜索域的 `nameserver-policy`，保留显式导入的同名 policy。默认 blacklist 和 rule
模式为这些域保留真实地址；whitelist 保留导入过滤语义。原路由器不作为公网 fallback。
无法作为原解析器使用的自身监听/loopback/fake 地址会被排除。
导入 DNS 中的 `system`（含旧 `dhcp://system` 写法）在接管开启时必须拒绝，
避免引擎递归解析自己；改用显式解析器，或明确关闭该协同功能。

## 独立的 Mac IPv6 路径

Mac 的系统 TUN 始终使用 `fdfe:dcba:9877::1/126`，不依赖 `dns.ipv6`、
下游 `transparent.tun_ipv6` 或上游原生 IPv6 检测。补丁核心必须保留显式
host TUN IPv6 配置。AAAA 只控制 DNS 答案，下游开关继续只控制 RA/BPF packet 路径。

统一生成 `route-address`：普通 IPv4 捕获减去 LAN/私有例外，公网 IPv6
`2000::/3`，整个 fake IPv6 `/64`，以及单独的 Tailnet peer/subnet 精确路由。
不能让 Tailnet `/128` 替代普通 IPv6，也不能经 IPSet 合并丢掉 `/32` / `/128`
优先级。link-local、multicast 和普通/下游 ULA 不加入 host 捕获。

目的 fake `/64` 路由与来源身份不同：Mac 规则只能匹配
`IN-TYPE,TUN + IN-NAME,DEFAULT-TUN + SRC-IP-CIDR,fdfe:dcba:9877::1/128`。
下游继续走 `opensurge-ipv6` / `IN-USER`，不得扩大 Mac source 匹配范围。

## 验证边界

启动与 Doctor 有限检查 DNS 捕获 IPv4、fake IPv4、fake IPv6 池两端和公网 IPv6
目标的实际路由接口。status/overview 只读轻量状态，不运行 route/scutil 扫描。
Doctor 另检查当前 DNS 所有权；Web 的 DNS 状态明确写为“启动时已校验”。

`make test` 覆盖路由矩阵、身份、DNS 读写失败、自动/手动设置恢复、重启与 boot
恢复。`make lab-test-tun-local-routing` 增加 macOS getaddrinfo 的随机新域名
fake A/AAAA 验收、关闭系统代理、UDP/TCP DNS 劫持、重启后重新接管与停止恢复。
标准 TUN gate 使用补丁核心。共享 L2 Lab 的 upstream 是无网络服务的虚拟 bridge，
因此该夹具关闭 Mac DNS 协同；真实 Mac DNS 由 isolated-LAN 本机模式门槛验证。
完整回归还需基础 Lab、TUN 和对应下游 IPv6 拓扑门槛。

这些门槛不证明某个订阅节点稳定，也不证明所有应用的独立 DoH、VPN DNS 或
内容过滤扩展都被覆盖。实际运行证据记录在测试 artifacts，不写入本页。
