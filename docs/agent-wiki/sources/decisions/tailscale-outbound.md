# Tailscale 出站契约

本页维护托管身份、目标与来源授权、配置合成、发现缓存及 Exit Node 角色；实现入口为 `internal/tailscale/`、`internal/mihomo/tailscale.go` 和 `internal/controlapi/`。

OpenSurge 把 mihomo `type: tailscale` 实现为一个受管 outbound，而不是第二个
系统 VPN 开关。mihomo 内部的 tsnet 负责 control plane、WireGuard、DERP、
NAT traversal 和 Tailnet DNS；OpenSurge 负责身份生命周期、目标与来源授权、
路由冲突检查、配置编译和 GUI。

## 稳定名称与身份

托管代理内部名称固定为 `open-surge/tailscale`，明确配置 Exit Node 时另生成
单成员 selector `open-surge/tailscale-exit`，其唯一成员是前者。Tailnet 目标规则
仍直接指向原始 outbound；面向用户的公网出口选择使用独立 selector。GUI 应显示
用户的 `display_name`，不应把内部名称当成产品文案。导入 profile 与 overlay
不得占用这两个保留名称。

Auth Key 是 write-only secret，保存在配置目录下权限为 `0600` 的独立文件。
`config.yaml` 只记录该文件路径，Control API 只返回是否已有 key。第一次
注册后，mihomo 的 `state-dir` 由 OpenSurge 持久化；重启、reload 和暂时停用
仍使用同一本地节点身份。

GUI 的首次设置会通过固定本机路径执行 `tailscale status --json`，只读发现
Tailscale App 当前可见的 Tailnet、MagicDNS、peer 精确 IP、在线状态、私网
`AllowedIPs` 路由和 `ExitNodeOption`。这些结果只是建议：用户勾选后才写入 draft，
MagicDNS 后缀不会默认开启，subnet route 与 Exit Node 也不会静默选择。发现调用
有短超时和输出大小限制；未安装、daemon 未连接或解析失败都返回可降级状态，不能
阻止高级手动配置。

一次成功且 backend 为 `Running` 的发现结果会以 `0600` 缓存在 Control Store，内容
只包含 Tailnet 名称、MagicDNS 后缀、peer 地址/能力和子网建议，不包含 Auth Key 或
托管节点 state。之后本机 Tailscale App 断开或 `status --json` 失败时，GUI 可以继续
使用该快照配置 OpenSurge，但必须明确标记“上次发现”，把在线状态显示为历史状态，
并重新按当前系统路由计算冲突；缓存不能被描述为当前连通性证据。这样推荐工作流是
先连接原生 App 完成一次发现，再断开原生 App 或关闭其 `accept-routes`，最后启动或
重载 OpenSurge，而不是要求两个独立 Tailscale 数据面同时接管相同精确路由。

GUI 使用卡片内的折叠设置，不把发现过程表现成必须依次完成的向导。面板显示信息来源
（本机 App、缓存或未发现）、本次检测/缓存时间和当前已知路由风险；展开时检测一次，
也允许操作者随时重复检测。这里不是持续监听，文案不得使用“实时”暗示。普通双客户端
连接只给建议，只有所选子网命中当前系统中的精确 foreign `utun` 路由时才阻止运行中
应用。PUT 应用端仍在 lifecycle lock 内重新检查系统路由，浏览器快照不能成为最终门禁。

macOS 的 Tailscale App 会根据调用环境在 GUI 与 CLI 模式之间选择；OpenSurge
Control API 由 LaunchAgent 启动，没有终端环境。因此执行 `status --json` 时必须为
子进程显式设置 `TAILSCALE_BE_CLI=1`，否则 Tailscale 可能把 GUI 启动错误写入
stdout，导致 JSON 解析失败。

本机 App 的发现结果不是托管节点的授权证明。两者不共享 identity、Auth Key 或
state，托管节点仍须用 write-only Auth Key 单独注册，并在应用后用实际访问验证
ACL、设备审批和路由可用性。官方 Tailscale 控制平面可从 GUI 打开 Keys 页面；
Headscale 的 preauth key 入口由各部署自行提供。

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
RFC1918 地址就被 Tailscale 接管。所有允许规则之后还会为这些明确目标生成
`REJECT`，避免未授权来源落入普通 `DIRECT` 后被本机原生 Tailscale App 的系统
路由接走。命中允许规则后直接选择 Tailscale outbound；节点离线时 fail closed，
不增加 `DIRECT` 回退。

完整配置校验必须先按当前 LAN 和网关模式准备设备策略 bundle，再校验
Tailscale 设备授权。网络配置保存会清除旧 bundle，避免沿用修改前的拓扑；
不能把“尚未加载”误判为缺少 `device_policy.file`，也不能只检查路径而跳过
指定设备是否有效的校验。`ValidateRuntime` / `LoadRuntime` 仍跳过可变的期望
策略文件，保证停止、状态和已应用策略操作不被无效草稿阻塞。

`allow_mac` 复用本机路由编译器的同一组入口身份。当前系统 TUN IPv6 固定为
`IN-NAME,DEFAULT-TUN` 与 `fdfe:dcba:9877::1/128` 的精确来源；不得扩大为
fake-IP / 下游子网，或把 `opensurge-ipv6` 的设备流量识别成本机。完整定义见
[本机流量模式](local-mac-routing-modes.md)。

Mihomo 一旦配置 `route-address`，就会替换 Darwin TUN 的默认自动路由集合。编译器
因此先重建普通公网捕获范围并预先扣除 LAN、RFC1918、loopback、multicast 等默认
排除范围，再追加精确 peer CIDR 和 subnet route；custom route 模式不再同时生成
`route-exclude-address`，避免 IP set 合并时吞掉用于覆盖原生 Tailscale route 的精确
`/32` 或 `/128`。这只改变系统 TUN 捕获，实际进入 Tailscale outbound 的流量仍必须
同时命中明确目标与授权来源规则；不得把整个 `100.64.0.0/10` 配置为 peer 目标。
远端 subnet route 必须是 private IPv4 或 ULA，必须启用 `accept-routes`，并且
不得与 OpenSurge 当前 LAN 重叠。

发现结果还要对每条已发布子网执行系统路由查询。只有当前选中路由的前缀与待安装
CIDR 完全一致、接口为 `utun*`，且接口不是 OpenSurge 当前系统 TUN 时，才报告
原生 Tailscale App 冲突；不能把原生 Exit Node 的宽泛默认路由误报为精确子网冲突。
运行中的网关在调用 helper 前以 `tailscale_route_conflict` 拒绝应用，GUI 显示路由、
接口和解除方法。网关停止时允许保存目标值，但必须提示在下次启动前解除冲突。
OpenSurge 不得自动改写原生 Tailscale App 的 `accept-routes` 状态。

## 同一远端 Mac 兼任 Exit Node 与 Subnet Router

远端 Mac 可以同时提供公网 Exit Node 和子网路由。访问它所在 LAN 的
SOCKS5、NAS 或 HTTP 服务时，应验证精确 subnet route；公网出口选中该 Mac，
不等于已验证子网服务。命中 Tailnet 目标的连接日志显示 `open-surge/tailscale`
是预期行为，不要求经过公网 selector `open-surge/tailscale-exit`。

当本地 LAN 与远端 LAN 的 IPv4 网段重叠时，直接访问远端服务的 IPv4 可能
命中本地设备。可使用 Tailscale 4via6 精确映射：由远端 Subnet Router 发布并
批准映射 IPv6 路由，OpenSurge 接受该路由、授权来源并配置 MagicDNS 后缀。
该路径需要 `dns.ipv6: true` 以允许相关 IPv6 解析，不要把它与为下游设备
发布 RA 或强制启用下游 IPv6 混为一谈。下游客户端可先以 IPv4/fake-IP
流量进入 OpenSurge，再由内嵌 tsnet 访问映射地址。

验证必须区分 Mac 本机入口和下游设备入口。在 Mac 上访问成功不能替代
手机验收；手机需要返回真实 SOCKS5/HTTPS 结果，并与 OpenSurge 日志中的
手机源 IP、映射目标和 Tailscale action 对应。需要独立证明特定 Mac 转发时，
还应核对路由发布者，或在该 Mac 的 LAN 接口抓取目标端口的包头。
详细操作和已验证边界见
[真实 LAN 的 4via6 子网 smoke](../validation/evidence-map.md#tailscale)。

## Tailnet 与 Exit Node 角色

Tailnet-only outbound 只为上述明确目标服务，不得成为 device policy 的
公网出口候选。它也不使用普通公网 `generate_204` 做健康检查；健康状态
显示为 `available_on_demand`。mihomo 只在 outbound 首次收到请求时启动
Tailscale 节点；OpenSurge 在网关启动、重载和 mihomo 恢复后发送一次最佳努力
预热请求，把大部分 lazy-start 成本提前，但产品文案仍须说明首次业务访问可能重试。

这次预热只等待 delay API 请求写出（最多两秒），不再等待完整健康响应。长驻 Helper
在后台读取并关闭响应，使用独立且有超时的 context，不能随着网关操作返回而取消。
短生命周期 CLI 也必须先写出请求才返回，不能只启动 goroutine 后立即退出。此实现依赖
锁定的 mihomo delay handler 用 `context.Background()` 派生检查上下文，以及 tsnet
自身的 lazy-start 生命周期；升级内核时需重新核对这两个边界。

Tailnet 仍使用原来的 4 秒预热预算，Exit Node 仍使用原来的 15 秒预算，手动出口检测不
缩短。操作通知只说明“预热已发起”或“未能发起”，不冒充出口已联网。连接过程中允许
用户选择出口，正常 selector 持久化保留的是选择，不是探测失败；本次优化不改选择、
拨号、内核重试或故障恢复语义，也不新增探测通过才能选择的限制。

配置明确的 `exit-node` 后，编译器生成可见的
`open-surge/tailscale-exit -> open-surge/tailscale` selector。该组会加入 device
policy 候选；当 `allow_mac` 为真时也会加入 `open-surge/mac-global`，供用户在
Mac 本机全局出口中显式选择。最终运行配置还会在导入 profile 与附加配置已经合成后，
为所有用户定义的 `type: select` 组追加该 Exit Node 候选，包括只有 `use`、
`include-all` 或隐藏显示的用户组；不向自动测速/故障转移/负载均衡组、Mac 内部组、
设备专用组和 Exit 包装组批量注入。原始订阅与附加配置源文件保持不变。共享的 YAML
anchor/alias 在注入前展开为独立运行节点，避免手动组的新增成员泄漏进自动组。
导入 profile、全局附加配置与 Tailscale 都是独立可空的输入；例如只有全局附加配置时，
其中新增的手动 `select` 组仍会在停止态策略预览和 Web GUI 最终启动事务中获得同一个
Exit 候选（前提是 Tailscale Exit Node 本身已启用），而没有 Tailscale 时则保持原组成员。

本次只追加 `proxies` 成员并去重，不修改 mihomo 的 Provider 展开、过滤、默认选择或
持久化规则。有有效历史选择时仍由内核恢复；没有有效选择时使用内核的第一候选，
所以原本只有 `use` 的组可能默认选择新增的 Exit Node。`include-all`、`filter`、
`exclude-filter` 与 `exclude-type` 保留原样，因此排除条件仍可能隐藏新增候选。
删除或停用 Exit Node 后，下一份最终配置不再包含自动追加的成员；普通用户组原先
选中的 Exit 消失时仍按内核原生规则选择，不承诺保留或阻止这次回退。设备专用出口
仍保留原有的“先换掉设备出口再停用”校验。

用户组的选择由所有命中流量共享，Mac Rule 与跟随网关规则的设备使用同一项选择。
这些公网候选不受 `allow_mac` / `allowed_devices` 限制；这些授权字段仍控制明确
Tailnet 目标的前置允许/拒绝规则，不能因为增加候选而扩大 Tailnet 目标访问。
原始 `open-surge/tailscale` 仍作为兼容目标被校验器接受。
Exit Node 使用不依赖 DNS 的公网固定 IP 探测；自动预热和 GUI 手动检测会给移动网络/
DERP 慢链路更宽的探测预算，但这只改变检测，不改 mihomo 普通 TCP 连接的全局拨号
超时。公网探测只证明该 Exit Node 到一个检测目标的单次可达性，不能把 GFW、目标站点
限制或 Android 用户态 Exit Node 的波动误报成 Tailnet 节点离线。离线时不自动回退到
`DIRECT`。
自动发现的 Exit Node 保存值应优先使用 peer 的 Tailscale IPv4，其次才是 IPv6、
MagicDNS 完整名称或主机名；这避免嵌入节点启动阶段额外依赖名称匹配。手动名称仍受
支持，编辑已保存配置时若能在当前或缓存发现结果中唯一匹配 peer，则 GUI 会把它
规范化为该 peer 的稳定 Tailscale IP。IP 规范化不代表 Exit Node 已获后台批准，
也不替代 `autogroup:internet`/ACL、远端转发能力和真实公网探测。

## 当前能力边界

这个集成不运行独立 `tailscaled`，也不与 Tailscale App 共享本地 state。
它是 outbound-only：不发布 OpenSurge LAN route，不让 OpenSurge 成为 subnet router，
不通过该节点提供入站服务。Headscale 使用同一实现，只替换
`control-url`。


## 验证入口

单元测试覆盖配置、密钥不回显、授权、路由冲突与合成；patched Mihomo `-t` 仅证明 YAML 可被当前核心接受。真实 peer/MagicDNS、来源授权和密钥清理见 [Tailscale Lab](../validation/test-gates.md#Tailscale-出站门槛)；远端子网与公网 Exit Node 的证据必须分开，参阅 [4via6 smoke](../validation/evidence-map.md#tailscale)。
