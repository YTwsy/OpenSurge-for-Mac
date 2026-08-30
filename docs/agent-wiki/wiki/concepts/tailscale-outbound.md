# Tailscale 出站

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

`allow_mac` 必须复用本机 Rule/Global/Direct 编译器的同一组入口身份，
不能另写一套宽泛的 IPv6 来源匹配。对系统 TUN 的 IPv6，规则同时
限定 `IN-NAME,DEFAULT-TUN` 和当前实际生效的单个 `/128`：DNS fake-AAAA
路径使用 `fdfe:dcba:9876::1/128`，显式 TUN IPv6 则使用
`fdfe:dcba:9877::1/128`。不得扩大为 fake-IP `/64`、下游 `/64`、`fc00::/7`，
也不得把 `opensurge-ipv6` packet listener 的下游设备流量当成 Mac 本机。

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

## Tailnet 与 Exit Node 角色

Tailnet-only outbound 只为上述明确目标服务，不得成为 device policy 的
公网出口候选。它也不使用普通公网 `generate_204` 做健康检查；健康状态
显示为 `available_on_demand`。mihomo 只在 outbound 首次收到请求时启动
Tailscale 节点；OpenSurge 在网关启动、重载和 mihomo 恢复后发送一次最佳努力
预热请求，把大部分 lazy-start 成本提前，但产品文案仍须说明首次业务访问可能重试。

配置明确的 `exit-node` 后，编译器生成可见的
`open-surge/tailscale-exit -> open-surge/tailscale` selector。该组会加入 device
policy 候选；当 `allow_mac` 为真时也会加入 `open-surge/mac-global`，供用户在
Mac 本机全局出口中显式选择。创建该组不会自动切换 Mac 或任何设备，也不修改
任意导入订阅的策略组。原始 `open-surge/tailscale` 仍作为兼容目标被校验器接受。
Exit Node 使用公网延迟探测，但离线时不自动回退到 `DIRECT`。

## 当前能力边界

这个集成不运行独立 `tailscaled`，也不与 Tailscale App 共享本地 state。
它是 outbound-only：不发布 OpenSurge LAN route，不让 OpenSurge 成为 subnet router，
不通过该节点提供入站服务。Headscale 使用同一实现，只替换
`control-url`。

## 验证边界

单元测试覆盖配置 round-trip、密钥不回显、路由冲突、规则顺序、imported/
managed 编译与 rollback。使用项目补丁构建的 mihomo 运行 `-t` 只能证明
最终 YAML 被当前内核接受。

`make lab-test-tailscale` 是当前专门的真实 Tailnet Virtual Lab 门槛。它使用一台不
连接 `omg0` 的持久 Lima peer、Mac 原生 Tailscale App 和两个 socket_vmnet 下游客户端，
验证本机发现、精确 peer IPv4、完整 MagicDNS 名称、TCP/UDP request-response、单设备
授权与其他设备 `REJECT`。peer fixture 还要求成功流量来自 managed tsnet 的同一地址，
并且不同于 Mac 原生 App 身份；Mihomo log 必须同时记录对应
`open-surge/tailscale`/`REJECT` action，因此不能用 Mac 已有的 `/32` 原生路由制造
假阳性。网关启动前还要求该 peer 的原生 route 确实指向一个 `utun`，让负向边界
建立在可观察的现成旁路上，而不是只假设旁路存在。

peer VM 的 underlay 仍通过 Lima NAT 和 Mac 当前上游，启动网关后也可能经过系统 TUN。
门槛会拒绝 Mac 正在使用原生 Exit Node 的环境，避免不必要的嵌套出口；普通 host
underlay 不作为应用路径证据。Auth Key 只从仓库外 `0600` 文件读取，临时 guest key、
生成的含密钥 YAML 和原始 topology log 在退出时清理，artifact 只保留脱敏规则与布尔
证据。Lab 会在 root 网关写入前把 `mihomo.yaml` 预创建为 `0600`，写入后再次断言，
避免临时 Auth Key 因默认 `0640` 运行文件权限暴露给组用户。

首次设置需要两个节点注册动作，不强制使用两枚长期不同的 key：one-off key 各自只能
注册一次；reusable、非 Ephemeral key 可以让两个文件变量指向同一个受保护文件。
peer 的 Lima 磁盘和 managed tsnet state 都会持久化，所以普通复跑无需 key；reusable
key 主要用于删除本地身份后的自动重建。

当前门槛不覆盖 subnet router、Exit Node 公网出口、Headscale 或真实远端 LAN。这些
路径仍必须增加各自受控 fixture 或真机证据，不能从 peer/MagicDNS 门槛外推。
