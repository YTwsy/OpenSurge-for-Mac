---
title: Validation gates
kind: source
status: seed
---

# 验证门槛

`make test` 是快速默认验证门槛。它运行 `go test ./...`，也是当前 CI 级别
检查。

`make lab-test` 是本地 host-network 门槛，服务于高风险网关变更。它在隔离的
socket_vmnet-backed LAN 中，用 Lima 客户端测试真实 macOS gateway，并检查
DHCP、DNS、ICMP/NAT、直连 HTTPS、通过 mihomo `mixed-port` 的显式代理
HTTPS，以及清理行为。

`make lab-test-tun` 是透明代理门槛。它会启用 `transparent.mode: "tun"`，
保持客户端没有显式代理配置，并要求无显式代理的 HTTPS 请求出现在
`mihomo.log` 的透明 TUN 路径中。

`make lab-test-tun-imported-profile` 是 imported profile overlay 的 TUN 门槛。
它使用 `tests/lab/mihomo-profile.imported-tun.yaml`，保持规则为 `MATCH,DIRECT`，
证明 imported profile 可以进入透明 TUN lab 路径。

`make lab-test-tun-imported-egress` 是 imported provider + policy-select 的 TUN
出口切换门槛。它使用本地 HTTP provider 注入 `egress-proxy`，通过
`omg policy-select` 把 `TunEgress` 从 `DIRECT` 切到受控 HTTP CONNECT proxy，并
要求 `mihomo.log` 中的 TUN 目标连接和受控 proxy 日志同时反映切换结果。这个门槛
不证明真实订阅节点、真实远端出口 IP 或 real-device/same-LAN 兼容性。

## 什么时候必须跑 lab

宣称下列改动具备 runtime 覆盖前，应运行 `make lab-test`：

- DHCP 或 DNS 行为；
- mihomo 进程启动或配置渲染；
- pf/NAT 规则；
- IPv4 forwarding 或 rollback 行为；
- 网关生命周期清理；
- lab 拓扑或测试脚本；
- runtime traffic defaults。

宣称透明代理路径被验证前，应运行 `make lab-test-tun`。

## 运行前置条件

lab 的 root-required 步骤依赖当前终端会话里的 sudo 缓存。`sudo -v` 和
`make lab-test` / `make lab-test-tun` 应在同一个 TTY 里连续运行；如果 agent 在
不同 exec 会话里刷新 sudo，脚本的 `sudo -n` 预检查仍可能失败。

虚拟 LAN lab 和真实设备 smoke 默认都使用 `192.168.50.1/24`。运行 lab 前，
这个地址只能存在于 lab 的 vmnet bridge 上。如果 `en7` 等 real-device 下游接口
仍保留 `192.168.50.1`，macOS 可能把 `192.168.50.0/24` 的回程路由选到错误接口，
表现为 `dig @192.168.50.1 example.com A` timeout，而 dnsmasq 日志仍显示收到了
查询。先运行 `make real-device-stop`，或手动删除重复地址。

## TUN 验收信号

当前 `make lab-test-tun` 的关键信号是：

- 客户端 helper 走 transparent 子命令，而不是显式代理测试；
- 客户端不依赖显式代理配置完成 HTTPS 请求；
- 脚本等待 `mihomo.log` 中出现 `--> <host>:443`；
- 成功时输出类似 `transparent TUN log observed for <host>:443`；
- 测试结束后停止 gateway，并确认 `runtime/lab/state.json` 被移除；
- artifacts 被写入 `artifacts/lab` 以便失败后排查。

`make lab-test-tun-imported-egress` 还应看到：

- `omg providers --format json` 中出现 `tun-egress-provider` 和 `egress-proxy`；
- `TunEgress[DIRECT]` 阶段受控 proxy 没有收到 `CONNECT <host>:443`；
- 执行 `omg policy-select --group TunEgress --policy egress-proxy` 后，`mihomo.log`
  出现 `using TunEgress[egress-proxy]`；
- 受控 proxy 日志出现 `CONNECT <host>:443`。

如果使用历史 lab 结果或人工观察提到 fake-IP DNS 行为，要明确它不是当前脚本
里唯一的直接断言。

## 结论纪律

如果只跑了单元测试，就只说单元测试通过。除非实际运行对应 lab gate，否则不
要暗示已经验证 host-network、root-required 或 transparent-proxy 行为。
