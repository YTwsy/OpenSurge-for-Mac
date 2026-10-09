# 下游 IPv6 接管契约

本页维护实验性下游 IPv6 的拓扑前提、packet path、身份与生命周期。支持范围和实际验收结果分别维护；历史结果见 [证据入口](../validation/evidence-map.md)。

OpenSurge 的下游 IPv6 接管支持三个拓扑，均要求 `transparent.mode: "tun"`。共享 L2 必须显式确认
`transparent.ipv6_shared_l2_ready: true`，否则配置 fail closed：

- `isolated_lan`：OpenSurge 自动发布 RA/SLAAC/RDNSS；
- `same_wifi_dhcp`：OpenSurge 成为全 LAN IPv6 提供者，操作者必须先关闭主路由
  RA/DHCPv6，或使用 RA Guard 消除竞争默认路由；
- `same_lan`：选择性手工接入，不广播 RA。客户端使用
  `fdfe:dcba:9878::/64` 中的唯一地址，默认网关和 DNS 都指向 Mac 接口的
  link-local IPv6，并移除该设备原有的主路由 IPv6 默认路由。

这个布尔值表达操作者已经知晓并完成共享 L2 前置条件，不是可持续启停的运行时功能。
Web GUI 使用明确的知晓复选框呈现它；切换拓扑时清除旧确认。

用户控制面保持两个独立设置：

- `dns.ipv6` 控制 mihomo DNS 是否回答 AAAA，并在开启时使用
  `fdfe:dcba:9876::/64` fake IPv6；
- `transparent.tun_ipv6` 取 `off | auto | always`，控制是否建立下游 IPv6 网关、
  DNS 和用户态透明数据面。`auto` 只在上游接口同时有公网全局
  IPv6 地址（排除 ULA）和 IPv6 默认路由时生效；`always` 强制建立下游路径。

两个设置独立：`off` 关闭下游接管；只开启接管仍可接入 IPv6 字面地址流量，
但普通域名不会因此获得 OpenSurge DNS 的 AAAA 答案。

下游 IPv6 不从物理接口路由进 macOS 系统 utun。root broker 在下游 Ethernet 的
BPF 上读取入站 IPv6 Ethernet frame，把 L3 packet 和 source MAC 通过权限为 0600
的 Unix datagram 交给打补丁的 Mihomo `opensurge-packet` listener。listener 用
sing-tun gVisor 处理 TCP/UDP，把 MAC 映射为内部 `device:<id>` InUser，再复用既有
设备规则和 outbound。返回包沿 Unix datagram 回到 broker，并按已观察的
IPv6-to-MAC 邻居表写回 Ethernet。QUIC 由 UDP/443 路径承载；这个实现不宣称代理
ICMPv6、ESP 或任意非 TCP/UDP 协议。

patched listener 当前显式设置 `DisableICMPForwarding: true`。这不影响本机链路上的
RA/SLAAC、RDNSS 与 Neighbor Discovery；它们由 dnsmasq、macOS 和客户端协议栈处理，
三个拓扑门槛会通过地址、默认路由、DNS 与真实双向 TCP/UDP/HTTP3 间接验证这些控制面
前置条件。不要仅为“ping 可用”打开 gVisor ICMP forwarding：常见代理 outbound 没有
对应的 ICMP 语义。若以后把 ping/traceroute 或跨出口 ICMPv6 定为产品需求，应先定义
每类 outbound 的映射与 fail-closed 行为，再新增专门 Lab。Packet Too Big / PMTUD 也应
作为独立的 MTU 与大包门槛验证；当前 HTTP/3 fixture 只证明基线 QUIC/HTTP3 流量，不
证明所有 PMTUD 场景。

安装包将 broker 安装为 `<OpenSurge root>/bin/opensurge-network`。新安装生成的配置
使用该绝对路径；升级保留的旧配置可能仍使用短名称，因此 broker 解析在 `PATH` 和当前
可执行文件旁边均未命中时，还要根据 `runtime.dir` 检查同一安装根目录的 `bin`。root
Helper 的 launchd `PATH` 不包含产品目录，不能依赖它偶然找到 broker。

内部身份不能使用设备策略组的 `device/<id>` 字符串，因为 Mihomo 的 `IN-USER`
规则把 `/` 当成多个用户名的分隔符。策略组继续使用用户可见的
`device/<id>/...`，packet listener 和对应规则使用单值 `device:<id>`。

`same_wifi_dhcp` 中的 `gateway_target: upstream_router` 是 IPv4-only 绕行。dnsmasq
向该设备下发主路由 IPv4 gateway/DNS，编译器不生成代理 selector 或普通设备规则；
若下游 IPv6 开启，packet listener 仍保留它的 MAC→`device:<id>` 映射，并在所有其他
规则前生成 `AND,((IN-TYPE,TUN),(IN-USER,device:<id>)),REJECT`。packet listener 会把
fake-AAAA 目标恢复为域名，因此不能依赖 `IP-CIDR6,::/0`；TUN + InUser 组合只命中
该 IPv6 packet path。其他设备继续使用
正常 IPv6 策略。控制面必须描述为“IPv6 出站已阻止”：共享 L2 的设备仍可能收到
SLAAC/RDNSS；若主路由 RA 未关闭或未被 RA Guard 消除，设备甚至可能完全绕过
OpenSurge 的 packet path，OpenSurge 无法按设备阻断那条链路。

dnsmasq 在 `isolated_lan` 和 `same_wifi_dhcp` 发布 `fdfe:dcba:9878::/64` 的
SLAAC/RA 和默认路由，并把下游接口的 link-local 地址发布为 RDNSS。RA 不设置
`high`/`low`，使用 RFC 4191 与 dnsmasq 的默认 Medium router preference。
`same_lan` 只启动 ULA gateway alias、IPv6 DNS listener、BPF broker 和 patched
Mihomo，不生成任何 RA 配置。`fdfe:dcba:9878::1` 保留为 Mac 的下游 gateway alias
和 dnsmasq 监听地址，但共享 L2 的手工客户端不依赖它完成邻居发现；默认网关和 DNS
都使用 Mac link-local 地址。
受控 vmnet bridge 验证中，手工客户端直接使用 ULA alias 作为 DNS 时邻居发现未可靠
完成、查询未到达 dnsmasq；link-local DNS 路径通过，因此 UI 与验收门槛不得再把 ULA
listener 作为旁路由客户端填写值。
Mihomo 系统 TUN 地址使用独立的
`fdfe:dcba:9877::1/126`。三段前缀必须保持互不重叠，fake IPv6 不能被本地 ULA
直连规则吞掉。

启动顺序是 Mihomo listener、BPF broker、下游 IPv6 gateway alias、dnsmasq RA；
停止顺序是 dnsmasq、router/prefix lifetime-zero withdrawal、alias removal、broker、
Mihomo。broker PID/fingerprint 必须在修改接口前写入 runtime state。`ipv6_ra_effective` 区分自动 RA 与不发送 RA 的手工路径；中间失败统一 rollback，清理失败保留 state 供重试。IPv6 source
地址被第二个 MAC 使用时保持第一次映射并拒绝冲突，不静默改绑。

`always` 不等于凭空获得公网 IPv6。上游没有原生 IPv6 时，fake IPv6 目标仍可由
支持相应域名/UDP 的代理出口承载；`DIRECT` 到真实公网 IPv6 地址仍会因为没有上游
IPv6 route 而失败。HTTP-only 代理也不能承载 UDP/QUIC。

## 实现来源

事实来源：

- `internal/ipv6packet/`
- `patches/mihomo/overlay/listener/opensurge_packet/`
- `internal/macosipv6/`
- `internal/dhcp/template.go`
- `internal/gateway/manager.go`
- `internal/mihomo/device_policy.go`
- `tests/lab/lab.sh`
- dnsmasq RA、RDNSS 与 `--ra-param`：<https://dnsmasq.org/docs/dnsmasq-man.html>
- RFC 4191 Medium default-router preference：<https://www.rfc-editor.org/rfc/rfc4191.html>

## 控制面与路由观察

Web GUI 将这两个设置组合在“下游 IPv6”卡片中，但不改变配置语义。三个拓扑在 TUN
开启时都提供 `off / auto / always` 与 AAAA 控件。共享 L2 还提供
`ipv6_shared_l2_ready` 前置条件确认；界面应使用“我已知晓”的复选框，而不是把它表现为
持续启停功能的滑动开关。切换拓扑时必须清除旧确认，不能沿用。旁路由页面
另有 IPv4/IPv6 填写速查，动态显示所选 Mac 接口的 link-local 默认网关。卡片应优先
解释设备会获得的地址、默认路由和 DNS，再显示运行时探测细节，不使用 `RA Override`
作为用户概念。

在 `same_wifi_dhcp` 运行期间，Mac 可能接收 OpenSurge 自己发布的 RA，并在下游接口上
形成以本机 link-local 地址为下一跳的 IPv6 默认路由。控制面的绕过提示必须区分这种
本机自有路由与外部路由器发布的竞争默认路由；只有前者不构成绕过 OpenSurge 的路径。
如果本机与外部默认路由同时存在，仍必须保留提示。旧恢复快照没有路由所有权字段时应
继续采取保守行为。

## 连接归属与验收

连接 owner 的专用 listener、MAC-backed applied 身份和守恒规则统一维护在 [连接观察契约](connection-observation.md)。三个 userspace IPv6 Lab 在 UDP 探测后核对受认证的 `/api/v1/connections`，输出 `ipv6-connection-observation.json`；命令、HTTP/3 和停止撤销范围见 [下游 IPv6 门槛](../validation/test-gates.md#下游-IPv6-门槛)。

2026-08-13 物理下游 Mac 的 TCP smoke 保留在 [真实设备记录](../validation/evidence-map.md#物理网络)，不能外推为 IPv6 UDP、GUA 或恢复验收。
