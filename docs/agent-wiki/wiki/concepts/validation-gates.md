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

## 结论纪律

最终报告必须明确说出实际运行了哪些命令。如果只运行了 `make test`，不要暗示
root-required lab 行为或 transparent routing 已经被验证。
