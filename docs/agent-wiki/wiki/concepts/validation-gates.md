# 验证门槛

判断某个变更需要什么证据时，先读这个页面。

OpenSurge for Mac 会修改 host networking，所以验证结论必须严格限定范围。
单元测试、静态检查和 virtual LAN lab 证明的是不同层级的事情。

## 快速门槛

运行：

```sh
make test
```

这个命令运行 `go test ./...`，是当前 CI 级别检查。对于不宣称真实 host-network
行为的小型代码、parser、renderer 和单元级变更，通常足够。

如果沙箱阻止 Go cache 写入，把 cache 放到 `/private/tmp` 下。

## Host-network 门槛

运行：

```sh
make lab-test
```

宣称 DHCP、DNS、mihomo 进程或配置生成、pf/NAT 规则、IPv4 forwarding、
rollback、网关生命周期清理、lab 拓扑或 runtime traffic defaults
被真实验证前，应运行这个门槛。

lab 会把 gateway 保持在 macOS 上，并用 socket_vmnet 网络中的 Lima 客户端
测试它。它验证 DHCP、DNS、ICMP/NAT、直连 HTTPS、通过 mihomo `mixed-port`
的显式代理 HTTPS，以及清理行为。

## 策略控制面门槛

运行：

```sh
make policy-control-test
```

这个门槛启动真实 mihomo 二进制和 OpenSurge CLI，但不启动 dnsmasq、pf、TUN，
也不需要 sudo。它用 imported profile fixture 验证 `policies`、`policy-select`、
`connections`、`providers`、`provider-update` 和聚合 `snapshot` 能通过 live
external-controller API 工作，并会重启 mihomo 证明 `profile.store-selected` 可以
恢复选中的策略。它还会启动本机 origin 和受控 HTTP CONNECT proxy，证明
`EgressSwitch` 可以把一次 mixed-port 请求从 `DIRECT` 切到受控代理。它适合策略组
控制、file/HTTP provider 状态读取和刷新、机器可读 CLI、mihomo API wrapper 和
`profile.store-selected` 相关改动；不要用它宣称 DHCP、DNS 下发、TUN 透明代理、
same-LAN、真实设备路径或真实远端代理出口已验证。

## 真实设备 smoke

虚拟 LAN lab 通过后，真实设备 smoke 的推荐入口是：

```sh
make real-device-start-off
make real-device-client-check
make real-device-stop
```

该入口由 `tests/real-device/smoke.sh` 提供，会生成本地
`runtime/real-device` 配置、构建当前 CLI、绑定下游接口、运行 root doctor、
启动 gateway，并做基础 DNS/API/listener 探测。它仍然需要用户在终端里输入一次
`sudo`，但把 root-required 步骤收敛到一个 sudo 会话；不要把它描述为免密 root
helper。

真实设备 smoke 能证明物理客户端接入下游 LAN 后，能从 OpenSurge 获得
DHCP/DNS 并通过 Mac 网关出站。它不替代 `make lab-test` / `make lab-test-tun`
作为代码改动的可复现 lab 门槛。

当前进度快照来自 `docs/agent-wiki/sources/validation/real-device-smoke.md`。
截至 2026-07-06 CST，本轮已经验证 explicit/off runner、TUN runner 和最小
proxy egress runner 可以在物理下游 LAN 启动，真实 Pixel 手机可以获得
`192.168.50.100-200` 范围租约且 router/DNS 为 `192.168.50.1`。手机侧无代理
直连 HTTPS/NAT、显式 `192.168.50.1:17890` HTTP proxy HTTPS、TUN 模式下无
显式代理 HTTPS，以及本机受控 upstream proxy 命中 `open-surge-egress` 均已完成
一次 smoke；Mac 侧能对应看到租约、DNS 查询、fake-ip 查询、`mihomo.log` 中的
客户端目标连接，以及受控代理日志中的 `CONNECT example.com:443`。`stop` 清理
范围包括释放下游接口上的 `192.168.50.1` 测试 LAN IP，避免后续 virtual LAN lab
把 `192.168.50.0/24` 回程路由选到真实设备接口。

explicit 模式的关键验收信号是：

- `make real-device-status` 显示 `Gateway: running`、`DHCP: running`、
  `mihomo: running`、`pf anchor: loaded` 和 `IP forwarding: enabled`；
- `leases` 中出现真实物理客户端，而不只是备用 AP 或小路由器；
- `dig @192.168.50.1 example.com A` 能从 Mac 或下游客户端获得响应；
- `curl --proxy http://192.168.50.1:17890 https://example.com/` 能通过
  mihomo `mixed-port` 访问 HTTPS。

只有运行 `make real-device-start-tun` 并在客户端无显式代理时观察到成功出站、
fake-ip DNS 响应和 `mihomo.log` 中的真实客户端目标连接，才可以宣称真实设备
TUN smoke 被验证。

本轮已通过的真实手机检查是：

- 无代理直连 HTTPS/NAT：手机 Wi-Fi HTTP proxy 关闭，HTTPS 页面成功加载，
  Mac 侧能看到该手机租约和 DNS 查询。
- 显式 HTTP 代理 HTTPS：手机 Wi-Fi HTTP proxy 设为 `192.168.50.1:17890`，
  HTTPS 页面成功加载；如果日志级别允许，`mihomo.log` 能看到来自手机 IP 的
  HTTPS 连接。
- TUN 透明模式：`make real-device-start-tun` 启动后，手机保持无显式代理，
  HTTPS 页面成功加载，并且 `mihomo.log` 中出现来自手机 IP 的目标连接。
- 最小 proxy egress：`make real-device-start-tun-proxy` 启动后，手机保持无
  显式代理，`https://example.com/` 成功加载；`dnsmasq.log` 中出现该手机的
  `example.com` fake-ip 查询，`mihomo.log` 中出现
  `example.com:443 match Domain(example.com) using
  open-surge-egress[real-device-egress]`，受控本机 HTTP CONNECT proxy 日志中
  出现 `CONNECT example.com:443`。

默认生成的 mihomo 配置仍然是 `MATCH,DIRECT`。已验证的 proxy egress smoke 只证明
最小受控 `upstream_proxy` 切片能把指定域名交给 `open-surge-egress` 和本机受控
代理；它不证明订阅规则、远端代理节点、策略组切换或出口 IP 变化。只有换成具有
独立出口的实际代理，并同时看到 `mihomo.log` 使用 `open-surge-egress`、受控代理
日志有对应请求、出口 IP 发生预期变化，才可以宣称远端代理出口路径被验证。

## same-LAN TUN smoke

同 LAN 默认网关 smoke 的入口是：

```sh
make same-lan-start-tun
make same-lan-start-tun-proxy
make same-lan-start-tun-imported-egress
make same-lan-adb-check
make same-lan-adb-check-imported-egress
make same-lan-stop
```

这个 gate 覆盖 Mac 和一台 Android 测试设备处于同一家庭或办公室 LAN 的场景。
第一版只允许 `gateway.mode: "same_lan"`、`dhcp.enabled: false` 和
`transparent.mode: "tun"`。OpenSurge 不在主 LAN 上发 DHCP；测试手机需要手动或由
外部 DHCP 配置把默认网关和 DNS 指向 Mac 的 LAN IP。

如果进入“同一个 Wi-Fi 关闭主路由 DHCP、由 OpenSurge 接管 DHCP/DNS”的测试切片，
先阅读 `tests/same-lan/WIFI-DHCP-RECOVERY.zh-CN.md`。恢复不是附属步骤，而是验收
的一部分：需要证明路由器 DHCP 已恢复、Mac Wi-Fi 回到 DHCP、至少一台客户端能重新
自动获取地址并上网。

这个高风险切片使用独立的 `gateway.mode: "same_wifi_dhcp"`，而不是放宽
`same_lan` 的 `dhcp.enabled: false` 不变量。入口是：

```sh
OMG_SAME_WIFI_DHCP_ROUTER_DHCP_DISABLED=confirmed \
OMG_SAME_WIFI_DHCP_PROTECTED_IPS=<static-ip-list> \
make same-wifi-dhcp-start-imported-egress
make same-wifi-dhcp-adb-check-imported-egress
make same-wifi-dhcp-stop
```

runner 不会改变路由器设置；`confirmed` 只是要求操作者在手动关闭路由器 DHCP 后明确
确认。它拒绝把 Mac gateway 或 `OMG_SAME_WIFI_DHCP_PROTECTED_IPS` 中的静态地址放进
租约池。ADB gate 要求 Android 的地址在池内且出现在 `omg leases` 与 DHCPACK 日志中，
并继续要求 DNS、无显式代理 TUN、provider/update、`TunEgress` 的 DIRECT 与
egress-proxy 切换，以及受控 CONNECT 证据。stop 检查 OpenSurge runtime/PF/listener/
helper 清理和 IPv4 forwarding 恢复；随后仍必须由操作者恢复路由器 DHCP 和客户端自动
获取。完整 runbook 在 `tests/same-lan/WIFI-DHCP-RUNNER.zh-CN.md`。

Web GUI 允许操作者跳过客户端验收，或在网关停止后保留 Mac 静态 IPv4 并结束状态机；
这些是交互流程的明确 waiver，不是 Lab / same-WiFi gate 的通过证据。凡是宣称 DHCP、DNS、
TUN 客户端路径或停止后自动获取恢复已验收，仍必须收集本节要求的真实证据，不能用
`client_validation_skipped` / `complete_static` 代替。

`make same-lan-adb-check` 是客户端证据入口。它应通过 ADB 采集 Android 默认路由、
DNS 查询、无显式代理探针结果，并回看 Mac 侧 `dnsmasq.log` 和 `mihomo.log`。只有
同时看到 Android 默认路由包含 `via <mac-lan-ip>`、DNS 查询走 Mac、客户端触发目标
连接、`mihomo.log` 中出现 TUN 路径目标连接，才可以宣称 same-LAN TUN smoke 通过。

基础 `same-lan-start-tun` gate 不证明主路由 DHCP 全局下发、所有设备兼容、IPv6、
DoH/Private Relay、UDP/QUIC、imported profile 或策略组切换。Android 镜像如果缺少
`curl`、`wget`、`nc`、`nslookup` 或 `dig`，应把结果记录为 ADB 客户端探针能力不足，
而不是把人工浏览器页面成功当成完整自动化证据。

same-LAN 的真实代理出口可以先用最小 `upstream_proxy` 切片验证，不必先导入完整
订阅。2026-07-09 已用 `api.ipify.org`、Pixel 测试手机和 LAN HTTP 代理完成这一
层：Android 默认路由经 Mac、Android 显式代理为空、`dnsmasq.log` 看到 Android 源
IP 查询 `api.ipify.org`、`mihomo.log` 显示
`Domain(api.ipify.org) using open-surge-egress[same-lan-http-egress]`，Android
浏览器显示的出口 IP 与 Mac 直接使用该 LAN 代理访问 `api.ipify.org` 的结果一致。

策略组切换需要单独验证。当前自动生成的 `open-surge-egress` 只有一个代理成员，只能
证明命中代理；要证明切换，应让 group 至少包含 LAN 代理和 `DIRECT` 两个候选，通过
`omg policy-select --config <path> --group <name> --policy <member>` 切换选中项后
重复出口 IP 探针。

same-LAN imported provider 策略切换的入口是
`make same-lan-start-tun-imported-egress` 和
`make same-lan-adb-check-imported-egress`。前者渲染 imported profile fixture，并启动本地
HTTP provider/受控 HTTP CONNECT proxy；后者要求 Android 无显式代理路径先命中
`TunEgress[DIRECT]`，再通过 `omg policy-select --group TunEgress --policy
egress-proxy` 切换后命中 `TunEgress[egress-proxy]`，同时受控 proxy 日志出现
`CONNECT <host>:443`。这个 smoke 证明 same-LAN TUN 下的 imported provider-backed
策略切换可以命中受控本地代理；它不证明完整订阅兼容性或真实远端出口 IP。

2026-07-10 已用一台人工操作的 Android（未使用 ADB）完成这个 gate：手机手动把
网关和 DNS 指向 Mac、保持无显式代理；同一运行期内，`example.com:443` 先记录为
`TunEgress[DIRECT]` 且受控 proxy 日志为空，切换后记录为
`TunEgress[egress-proxy]` 且出现 `CONNECT example.com:443`。两次浏览器访问均成功，
随后 `make same-lan-stop` 恢复了 PF、IPv4 forwarding、监听端口和 runtime state。

生成的 imported profile 路径相对 `runtime/same-lan/config-tun.yaml` 解析，必须写为
`./mihomo-profile.imported-tun-egress.yaml`。同时，helper 必须同时监听本地 HTTP
provider 与受控 CONNECT proxy 两个端口才可开始客户端探针。

## 透明代理门槛

运行：

```sh
make lab-test-tun
```

宣称透明代理路径被验证前，应运行这个门槛。它启用
`transparent.mode: "tun"`，保持客户端没有显式代理配置，并要求 HTTPS 请求
出现在 mihomo TUN 路径中。

运行前确认：

- `sudo -v` 和 lab target 在同一个终端/TTY 里连续执行。sudo ticket 不是跨
  agent exec 会话可靠共享的状态。
- `192.168.50.1` 只配置在当前 lab bridge 上。真实设备 smoke 也会使用这个地址；
  如果 `en7` 等接口残留 `192.168.50.1/24`，macOS 可能把 lab client 回程路由到
  错误接口，表现为 TUN DNS timeout。先运行 `make real-device-stop` 或删除重复
  地址。

当前重要验收信号：

- 客户端不依赖显式代理配置；
- 客户端 helper 运行 transparent 测试路径；
- `mihomo.log` 中出现透明 HTTPS 目标，例如 `--> example.com:443`；
- 成功时输出 `transparent TUN log observed for example.com:443`；
- gateway 被停止，`runtime/lab/state.json` 被移除；
- artifacts 写入 `artifacts/lab`。

修改 mihomo profile 导入或 OpenSurge gateway overlay 行为时，优先使用：

```sh
make lab-test-tun-imported-profile
```

这个门禁使用 `tests/lab/mihomo-profile.imported-tun.yaml` 启动 imported profile
配置，并保持规则为 `MATCH,DIRECT`。它证明 imported profile overlay 可以进入 TUN
lab 路径；它不证明外部代理出口或远端节点可用。

验证 imported provider 和策略组切换会影响透明 TUN 出口时，使用：

```sh
make lab-test-tun-imported-egress
```

这个门禁使用 `tests/lab/mihomo-profile.imported-tun-egress.yaml`，在 lab host 上
启动本地 HTTP provider 和受控 HTTP CONNECT proxy。脚本先让无显式代理客户端通过
TUN 访问测试 HTTPS host，并要求 `mihomo.log` 显示 `using TunEgress[DIRECT]` 且受控
proxy 未被使用；随后执行 `omg policy-select --group TunEgress --policy egress-proxy`
并重复透明请求，要求 `mihomo.log` 显示 `using TunEgress[egress-proxy]`，同时受控
proxy 日志出现 `CONNECT <host>:443`。它证明 imported provider-backed 策略选择能
改变透明 TUN egress path；它不证明真实订阅节点、真实远端出口 IP、same-LAN 或真实
设备兼容性。

## 每设备策略门槛

运行：

```sh
make lab-test-tun-device-policy
```

当改动 MAC 绑定 DHCP reservation、设备路由模式、每设备 selector 或设备规则覆盖的
数据路径时，使用此门槛。它使用两个 Lima VM，验证两个设备获得 `.101`/`.102` 固定
IPv4，先证明 `dedicated` 设备的 default selector 位于全局 `MATCH` 之前，再证明
`inherit_global` 设备没有 default selector 且走全局 `MATCH`；随后通过 reload 把后者改成
独立模式，验证两台设备可独立选择不同 TUN egress，并验证设备专属 IP `REJECT`。它还断言 applied policy snapshot/state digest、`omg devices` 的
`policy_identity_ready`/`lease_match` 对真实租约成立、desired 文件修改后的 drift，
调用真实 `omg reload` 后网关回到 running 且 desired/applied digest 收敛，并再次检查
selector 隔离；同时要求设备默认 selector 指向 HTTP-only outbound 时 UDP/443 命中
`REJECT` fallback 而非 fall through 到全局 `MATCH,DIRECT`。它证明设备身份、跟随与
独立模式、默认出口、安全 reload、UDP fail-closed 和覆盖规则的真实 LAN/TUN 数据路径。

大型 rule-provider、模板与 domain/IP/protocol/port 组合只改变配置编译时，
`make test` 提供相应覆盖；不需要为每条操作者定义的规则运行 Lab。当前设备身份
边界是 MAC 绑定 IPv4 DHCP reservation 加 IPv4 `SRC-IP-CIDR`，不是 IPv6 或 mihomo
内的 MAC 匹配。

2026-07-11 已在 P1-1..P1-5 修复（commit `7b14586`）后运行此门槛并通过：两个 VM
拿到 `.101`/`.102` 固定租约且 `omg devices` identity 就绪，UDP
`192.168.50.101 -> 1.1.1.1:443` 命中设备 `REJECT` fallback，两设备 selector 独立
切换，设备级域名 `REJECT` 生效，stop 后 `state.json` 清除。artifacts 在
`artifacts/lab/20260711-194621`。ARP/ICMP reservation 冲突探测只在 `same_wifi_dhcp`
模式激活，此 lab（`same_lan`/tun）未在运行时覆盖该路径，由单元测试覆盖。

真实 same-WiFi 双设备门槛是：

```sh
make same-wifi-dhcp-start-device-policy
make same-wifi-dhcp-adb-check-device-policy
make same-wifi-dhcp-stop
make same-wifi-dhcp-verify-device-policy-recovery
```

它要求两台真实客户端的 SSID Wi-Fi MAC、固定 reservation 和 ADB serial，启动前主动
证明不存在其他 DHCP OFFER；数据面验证两台设备 identity、独立 default selector、
设备规则 selector、准确源 IP 和 UDP REJECT。恢复门槛在操作者重新打开路由器 DHCP 后
要求主动 OFFER、Mac DHCP `server_identifier`、恢复的默认路由和两台客户端 HTTPS。
截至本实现落地时该 gate 尚未在本轮真机运行；因此 same-WiFi per-device 只能标记为
Experimental / cooperative IPv4，不能借用 virtual lab 的通过记录宣称已验收。

## same-WiFi 上游断链恢复门槛

Mac 与下游客户端共用同一个 Wi-Fi 接口时，普通连通性 smoke 不足以证明上游断链恢复。
需要在完整恢复手册可用的前提下，单独记录以下证据：

1. 断链前，连通性探针分别证明一个 DIRECT 国内目标和一个真实/受控代理目标可用，并记录
   Mihomo 的 rule、chain 与日志；
2. 人工让 Wi-Fi 断开并重新关联，确认接口、静态 IPv4、router 和 DNS 已恢复；
3. 若 Mihomo 路径持续失败，保存 macOS Wi-Fi 时间线和当前 `mihomo.log`，执行
   `sudo omg restart-mihomo --config <path>`；
4. 证明 dnsmasq PID、PF anchor、IPv4 forwarding、Mac 静态网络和 DHCP 接管恢复阶段在
   动作前后没有变化，只有 Mihomo PID 改变；
5. 重复 DIRECT 与代理出口探针，要求均恢复，并确认旧日志已归档；
6. 最后完成 same-WiFi stop 与路由器 DHCP、Mac DHCP、客户端自动获取恢复门槛。

`make test` 只证明独立恢复动作的状态事务和接口边界。普通 `make lab-test-tun` 使用虚拟
LAN，不能代替物理 Wi-Fi 断开/重关联证据；在上述真实门槛未运行前，不得宣称自动恢复或
根因已经彻底修复。

## 结论纪律

最终报告必须明确说出实际运行了哪些命令。如果只运行了 `make test`，不要暗示
root-required lab 行为或 transparent routing 已经被验证。
