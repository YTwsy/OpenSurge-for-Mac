# 验证门槛与结论范围

本页按改动说明检查入口、通过标准与结论边界。自动化断言由
[Makefile](../../../../Makefile)、[Lab runner](../../../../tests/lab/lab.sh) 和各测试维护，
不在这里复制完整断言清单。环境准备与清理见 [Lab 运行](lab-gates.md)，
历史执行结果经 [证据入口](evidence-map.md) 定向读取。

## 快速门槛

普通 Go 代码、解析与生成逻辑运行 `make test`（`go test ./...`）；Web 交互与文案按范围
运行 `make web-test`，前端源码变化同时 `make web-build` 更新嵌入资源。原生桌面运行
`make desktop-test`。这些通过不证明真实网络、原生界面或安装。Go cache 权限受限时可把
`GOCACHE` 指向 `/private/tmp` 的专用路径。

## Host-network 门槛

涉及 DHCP/DNS、进程、PF/NAT、IPv4 forwarding、rollback 或清理时使用 `make lab-test`。
通过要求 Lima 客户端取得 DHCP/DNS、完成 ICMP/NAT、直连和 mixed-port HTTPS，并恢复
被接管的主机资源。运行前读取 [Lab 前提](lab-gates.md)，每轮检查当次 artifacts，收尾执行
`make lab-down`、`make lab-status`。未完成清理不能记为完整通过。

## 策略控制面门槛

`make policy-control-test` 启动真实 Mihomo 与 CLI，不开启 DHCP、PF、TUN，也不需 sudo。
用于策略/Provider API、选择缓存、本机模式协调与出口名称失效处理。通过要求真实核心的
选择、重启持久化、provider 读取/更新与候选回退符合 [profile](../decisions/mihomo-profile-overlay.md)
和 [设备策略契约](../decisions/device-policy-overlays.md)，本机模式不泄露内部 selector，
HTTP-only Global 的 UDP fail closed。它不能替代下游身份、DHCP/DNS 或真实 TUN 数据面。

## 下游 IPv6 门槛

| 拓扑 | 入口 | 必须证明的范围 |
| --- | --- | --- |
| 独立下游 LAN | `make lab-test-ipv6-userspace` | 自动 RA/SLAAC/RDNSS、两设备 MAC/InUser 策略与停止撤销 |
| 同 LAN DHCP 接管 | `make lab-test-ipv6-same-wifi` | 共享 L2 前提、IPv4 绕行设备的 IPv6 REJECT、另一设备继续正常出站 |
| 手工旁路 | `make lab-test-ipv6-same-lan` | 手工 ULA、link-local 网关/DNS、不发布 RA及停止清理 |

三条门槛均要求真实 BPF packet path 的 TCP、UDP、QUIC-shaped carrier 与受控 HTTP/3-only
请求/响应，并核对 `/api/v1/connections` 的双栈设备归属。HTTP/3 不允许 TCP/HTTP2 fallback，
要求 DIRECT、SOCKS5 UDP 与 HTTP-only REJECT 分支各有对应证据。关闭后撤销拥有的路由、
alias、broker/socket 与 state；客户端允许按 RFC 4862 暂时保留 deprecated SLAAC 地址。

QUIC-shaped 数据只证明 carrier；受控 HTTP/3 不证明所有协议版本、0-RTT、迁移或公网代理
组合。Unix 注入单测不代替 BPF/RA。完整运行规则见 [IPv6 契约](../decisions/downstream-ipv6-takeover.md)。

真实订阅和公网出口是独立补充，不替代受控门槛：

```sh
sudo -v && \
  OMG_LAB_IPV6_REAL_PROFILE=/absolute/path/to/profile.yaml \
  make lab-test-ipv6-imported-egress
```

要求 `tun_ipv6: auto` 的原生上游条件成立，分别验证受测 VM 的域名 REJECT、DIRECT IPv6
HTTPS/UDP 和真实叶子节点的 fake-AAAA/IPv6 字面地址路径；节点先通过 SOCKS5 UDP
ASSOCIATE 探测。Mac 与 DIRECT 客户端回显须落在上游 GUA，不要求隐私地址逐 socket 相同。
下游仍是 ULA，DIRECT 由 Mihomo/gVisor 建立新 socket，不是运营商委派 GUA 的原样转发。

订阅仅复制到 `0600` runtime；artifact 只留脱敏证据，不能含订阅、完整配置、原始日志、
selector 输出或 cache，并扫描凭据/节点标识。确认 stop 成功后再删除 runtime secrets；
清理失败保留私有恢复材料并记录失败。

## 物理设备与同 LAN 入口

物理客户端的命令、通过标准、主动 OFFER 与完整恢复统一见 [物理网络验收](physical-network.md)。
确定性 Lab、人工客户端和 ADB 结果分别记录；不互相替代。

## 透明代理门槛

| 改动范围 | 入口 | 通过标准与限制 |
| --- | --- | --- |
| TUN 接入与 readiness | `make lab-test-tun` | 客户端无显式代理，HTTPS 出现在 TUN 日志，停止后资源与 state 清理 |
| imported profile 合成 | `make lab-test-tun-imported-profile` | imported overlay 在 TUN 下工作；fixture 的 MATCH,DIRECT 不证明远端代理 |
| provider 与策略切换 | `make lab-test-tun-imported-egress` | DIRECT 阶段代理无命中，切换后新请求命中 egress-proxy 且出现 CONNECT |
| prepared workspace 与 App 候选启动 | `make lab-test-policy-workspace` | 预览不改 desired，最终校验一次，准备态交接、选择缓存与两设备 TUN 出口/清理正确 |

受控 CONNECT proxy 的 DNS 与上游 socket 必须绑定真实上游接口，防止重入待测 TUN。
prepared gate 不证明原生 UI 或真实 Tailnet Exit Node；CLI 持久化配置路径由 imported-egress
覆盖。退出准备态的进程/cache 所有权与候选事务见 [profile 契约](../decisions/mihomo-profile-overlay.md)。

出口失效的准备态真实核心回归可以单独运行：

```sh
OMG_PREPARED_MIHOMO_BINARY="$PWD/runtime/tools/bin/mihomo" \
  go test ./internal/controlapi -run 'TestPolicyWorkspaceMissingDeviceEgressRealCore$' -count=1
```

它检查连续预览、回退与原选择恢复，仅开放随机 loopback controller，不接管主机网络。

标准 TUN Lab 关闭系统代理协同。若要证明解决 Network Extension 冲突，需在真实 Mac 上
保留冲突扩展，记录原 HTTP/HTTPS/PAC/自动发现状态，对比 TUN-only 与协同开关的目标应用
访问，再 stop 确认恢复。mock `networksetup` 和关闭协同的 Lab 不证明该兼容结论。

## Mac 本机模式隔离门槛

修改本机身份、Rule/Global/Direct、系统 DNS 或本机与下游隔离时，运行
`make lab-test-tun-local-routing`。通过要求本机选择改变而下游保留其设备/网关规则，
HTTP-only Global 的 UDP 拒绝，系统 TUN 的 IPv4/IPv6 精确来源不会匹配 packet listener。
开启 AAAA 的无原生 IPv6 场景也需真实探针；范围以 [本机路由](../decisions/local-mac-routing-modes.md)
与 [系统 DNS 契约](../decisions/local-system-dns-coordination.md) 为准。

使用发布同源的 patched Mihomo，不能退回引导用的上游 v1.19.27。系统 DNS 验收要求
`getaddrinfo` 随机新域名返回 fake A/AAAA、系统代理关闭、UDP/TCP DNS 捕获、Mihomo 重启
后再次接管和 stop 恢复原设置。结合下游 IPv6 门槛才支持完整的本机/下游隔离结论。

## 每设备策略门槛

reservation、模式、selector、规则或连接刷新数据路径使用 `make lab-test-tun-device-policy`。
通过要求两 VM 的跨第三段 `/22` 租约和 applied 身份成立，跟随/独立模式、出口切换、重载、
UDP REJECT、旧 LAN dormant 设备排除均符合契约；设备刷新只关闭目标设备，共享组刷新
只关闭实际经过该组的连接。Mac 的 mixed-port 样本不代替系统 TUN 身份验收。
规则集/模版仅改变编译时由 `make test` 覆盖，不为每条用户规则重跑 Lab。

真实双设备、主动 OFFER 与恢复命令见 [DHCP 接管验收](physical-network.md#局域网-dhcp-接管)。
身份是路由归属，不是防 MAC spoofing 认证；已知结果与未覆盖范围见 [证据入口](evidence-map.md#物理网络)。

## Tailscale 出站门槛

```sh
OMG_LAB_TAILSCALE_PEER_AUTH_KEY_FILE=/private/path/peer.key \
OMG_LAB_TAILSCALE_OPEN_SURGE_AUTH_KEY_FILE=/private/path/managed.key \
make lab-test-tailscale
```

首次注册 peer VM 与 managed tsnet：one-off key 使用两个仓库外 `0600` 文件；可复用且非
Ephemeral 的 key 可共用文件。身份持久化后复跑不再需要对应 key。Mac 原生 App 必须
Running 且未使用 Exit Node；peer 只使用 Lima NAT/control NIC，不能经待测下游网络。

通过要求 peer IPv4/MagicDNS 的 TCP/UDP 与来源授权都在真实 Tailnet/TUN 上成立：授权
请求来自同一托管身份，未授权请求 REJECT，不能借用本机原生 route。App 在线提示仅为
advisory；应用路径由 peer 观察与 Mihomo action 证明。停止清除 runtime，artifact 不含
Auth Key、Tailnet 地址、完整配置、原始日志或 tsnet state。完整断言由 Lab runner 维护。

这条门槛不证明远端 Subnet Router、公网 Exit Node、Headscale 或全部 NAT/DERP 组合。
这些路径需补充真实远端验收：

1. 远端 LAN 的 SOCKS5 服务须经已批准子网路由可达；IPv4 重叠时配置 4via6 目标。
2. 启用托管 Tailscale、接受路由与对应 MagicDNS 后缀，分别授权本机和下游。AAAA 与下游 RA 接管分开判断。
3. 分别从 Mac 与下游终端执行请求；`socks5h` 让远端服务解析目标站点，服务域名仍由客户端解析，`--noproxy ''` 排除代理例外。
4. 同时检查 SOCKS5 握手、TLS 校验、HTTP 结果、受测来源与 Tailscale action。若需确认具体远端转发节点，另取路由或抓包证据。
5. 公网 Exit Node 单独选择默认出口，移除上述显式 SOCKS5 代理后再验收 HTTPS、出口与日志；子网成功不替代默认出口。

角色和授权见 [Tailscale 契约](../decisions/tailscale-outbound.md)，实际结果见 [证据入口](evidence-map.md#tailscale)。

## same-WiFi 上游断链恢复门槛

需要物理 Wi-Fi 断开/重关联证据时，执行 [断链恢复方法](physical-network.md#物理上游断链恢复)。
单测仅证明恢复事务与锁，虚拟 LAN 不代替物理链路、睡眠或重启恢复。

## 合盖运行门槛

单元和打包检查覆盖 lease、marker 所有权、外部 SleepDisabled 拒绝接管及异常恢复。
正式声明仍需在安装环境按 Mac 型号、macOS 和电池/电源分别检查：启用后合盖继续运行，
关闭后恢复睡眠，Control Service kill、Helper kill 和系统重启后恢复，并记录 `pmset -g`
的 SleepDisabled。保持通风，不在包内测试。具体契约见 [睡眠接管](../decisions/lid-closed-sleep-prevention.md)。

## 桌面构建与安装门槛

打包脚本、生产 bundle、登录项、真实 PackageKit 与各 macOS/架构的检查统一见
[桌面与安装验收](desktop-smoke.md#安装验收)。实际覆盖与未覆盖见 [证据入口](evidence-map.md#桌面与安装)。

## 桌面交互

原生窗口、菜单栏、会话与固定能力按 [desktop smoke](desktop-smoke.md) 检查。
fixture、实际安装和 host-network 分别限定结论。

## 结果与维护

本页不累计执行日期或“已通过”报告。一次执行的版本、环境、实际检查、结果、证据位置
与未覆盖项写入提交/PR 或已有任务记录；已有记录结束后归档到 `tasks/finished_archived/`。
仅当新证据影响能力判断、暴露阻塞或实现变化要求复验时，更新证据入口和当前限制。
普通小改动不为记录几个命令新建任务或 validation 页面。
