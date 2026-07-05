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

explicit 模式的关键验收信号是：

- `make real-device-status` 显示 `Gateway: running`、`DHCP: running`、
  `mihomo: running`、`pf anchor: loaded` 和 `IP forwarding: enabled`；
- `leases` 中出现真实物理客户端，而不只是备用 AP 或小路由器；
- `dig @192.168.50.1 example.com A` 能从 Mac 或下游客户端获得响应；
- `curl --proxy http://192.168.50.1:17890 https://example.com/` 能通过
  mihomo `mixed-port` 访问 HTTPS。

只有运行 `make real-device-start-tun` 并在客户端无显式代理时观察到成功出站和
`mihomo.log` 中的真实客户端流量，才可以宣称真实设备 TUN smoke 被验证。

## 透明代理门槛

运行：

```sh
make lab-test-tun
```

宣称透明代理路径被验证前，应运行这个门槛。它启用
`transparent.mode: "tun"`，保持客户端没有显式代理配置，并要求 HTTPS 请求
出现在 mihomo TUN 路径中。

当前重要验收信号：

- 客户端不依赖显式代理配置；
- 客户端 helper 运行 transparent 测试路径；
- `mihomo.log` 中出现透明 HTTPS 目标，例如 `--> example.com:443`；
- 成功时输出 `transparent TUN log observed for example.com:443`；
- gateway 被停止，`runtime/lab/state.json` 被移除；
- artifacts 写入 `artifacts/lab`。

## 结论纪律

最终报告必须明确说出实际运行了哪些命令。如果只运行了 `make test`，不要暗示
root-required lab 行为或 transparent routing 已经被验证。
