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
