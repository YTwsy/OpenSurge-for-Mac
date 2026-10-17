# Mac 系统 DNS 与 IPv6 TUN 契约

本页维护系统 DNS 的读取、写入、恢复及 host TUN 路由范围。

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

订阅自身 DNS 的引用绑定发生在最终解析配置合成后，正式端点仍为受管的 1053，
不改变本页的系统 DNS 接管目的地址或恢复顺序。准备态如需自身 DNS，只开启同一核心的
私有 loopback DNS，不调用系统 DNS 协同；引用分类、歧义声明及就绪检查见
[profile 契约](mihomo-profile-overlay.md#dns-reference-composition-and-binding)。

## 文件式自定义 DNS 的双入口约束

来源页的自定义 DNS 先于系统 DNS 协同合成到同一份 Mihomo 配置，随后沿既有引用绑定和
生命周期处理。启用候选必须同时满足 TUN、自动路由、系统 DNS 协同及 dnsmasq 使用
`127.0.0.1#1053` 上游；不满足时可保存草稿，禁止应用，不自动修改网络设置。
自定义候选还会拒绝指回自身网关 DNS 监听的已知转发环；系统接管使用的公网目的地址
不等同于本地监听，显式引擎上游仍沿受管出站接口处理。

生效期间 dnsmasq 加入 `no-hosts`，系统 hosts 是否使用交给 Mihomo `use-system-hosts`；
自定义 hosts 非空时生成 `use-hosts: true`，同一草稿的显式 false 报冲突。关闭并应用后
恢复原 dnsmasq 行为，始终不修改 `/etc/hosts`。DHCP 的单标签、配置域及未来租约范围
继续由 dnsmasq 所有；自定义 hosts/policy/别名链的交集在应用前拒绝，不加入
`dhcp-ignore-names`，也不声称实现 DHCP 动态名称同步或 mDNS 接管。

保存草稿不改变运行态。通过既有启动或来源应用进行完整切换；来源应用的完整 reload
会重启所属 dnsmasq，避免沿用旧服务缓存。客户端 TTL 缓存仍由客户端管理。
Mac 和下游的 UDP/TCP、固定 A/AAAA、公司上游与真实连接验收仍待用户执行；
当前结构检查不能当作这两个入口的运行证据。详见
[自定义 DNS 契约](mihomo-profile-overlay.md#file-based-custom-dns)。

## 独立的 Mac IPv6 路径

`transparent.mac_tun_ipv6` 默认 `true`，旧配置缺省时保留原行为。开启时 Mac
系统 TUN 使用 `fdfe:dcba:9877::1/126`，不依赖 `dns.ipv6`、下游
`transparent.tun_ipv6` 或上游原生 IPv6 检测。关闭时显式生成 `inet6-address: []`
覆盖核心自带地址，并移除公网、fake IPv6 和 Tailnet IPv6 的系统 TUN 捕获路由及
本机 IPv6 来源规则；IPv4 和下游 RA/BPF packet 路径保持原行为。

该开关不改 DNS AAAA 或 mihomo 顶层 IPv6，也不禁用 macOS 原生 IPv6；关闭后
本机不能经 TUN 访问 fake IPv6 或 Tailnet IPv6，应用是否回退 IPv4 需实际验证。
保存后完整重启网关生效；单独重启 mihomo 使用 runtime state 中已应用的值。
Control API 旧客户端省略字段时保留当前设置。启动/Doctor 在关闭时仅检查 IPv4
路由，状态依据已应用的开关显示 disabled。

统一生成 `route-address`：普通 IPv4 捕获减去 LAN/私有例外；本机 IPv6 开启时加入公网 IPv6
`2000::/3` 与整个 fake IPv6 `/64`。Tailnet peer/subnet 精确路由中的 IPv4 始终保留，
IPv6 仅在本机 IPv6 开启时加入。
不能让 Tailnet `/128` 替代普通 IPv6，也不能经 IPSet 合并丢掉 `/32` / `/128`
优先级。link-local、multicast 和普通/下游 ULA 不加入 host 捕获。

目的 fake `/64` 路由与来源身份不同：Mac 规则只能匹配
`IN-TYPE,TUN + IN-NAME,DEFAULT-TUN + SRC-IP-CIDR,fdfe:dcba:9877::1/128`。
下游继续走 `opensurge-ipv6` / `IN-USER`，不得扩大 Mac source 匹配范围。

## 验证入口

启动与 Doctor 有限检查 DNS 捕获 IPv4、fake IPv4、fake IPv6 池两端和公网 IPv6
目标的实际路由接口。status/overview 只读轻量状态，不运行 route/scutil 扫描。
Doctor 另检查当前 DNS 所有权；Web 的 DNS 状态明确写为“启动时已校验”。

`make test` 覆盖路由矩阵、身份、DNS 读写失败、自动/手动设置恢复、引擎重启与跨 boot
恢复。标准 TUN gate 使用补丁核心。共享 L2 Lab 的 upstream 是没有 macOS 网络服务的
虚拟 bridge，因此该夹具关闭 Mac DNS 协同；真实系统解析由 isolated-LAN 的本机模式门槛
验证。完整网络回归还需基础 Lab、TUN 和受影响的下游 IPv6 拓扑门槛。

本机 DNS 与路由验收见 [本机模式门槛](../validation/test-gates.md#Mac-本机模式隔离门槛)。实际运行结果须从对应 artifacts 核对；这些门槛不证明订阅稳定性或所有独立 DoH、VPN DNS、内容过滤扩展。
