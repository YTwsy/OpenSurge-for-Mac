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

## TUN 验收信号

当前 `make lab-test-tun` 的关键信号是：

- 客户端 helper 走 transparent 子命令，而不是显式代理测试；
- 客户端不依赖显式代理配置完成 HTTPS 请求；
- 脚本等待 `mihomo.log` 中出现 `--> <host>:443`；
- 成功时输出类似 `transparent TUN log observed for <host>:443`；
- 测试结束后停止 gateway，并确认 `runtime/lab/state.json` 被移除；
- artifacts 被写入 `artifacts/lab` 以便失败后排查。

如果使用历史 lab 结果或人工观察提到 fake-IP DNS 行为，要明确它不是当前脚本
里唯一的直接断言。

## 结论纪律

如果只跑了单元测试，就只说单元测试通过。除非实际运行对应 lab gate，否则不
要暗示已经验证 host-network、root-required 或 transparent-proxy 行为。
