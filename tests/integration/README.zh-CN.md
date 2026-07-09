# 集成测试

简体中文 | [English](README.md)

自动化虚拟 LAN lab 位于 `tests/lab`。它使用真实的 macOS `pf`、`dnsmasq`
和 `mihomo` 实现，并配合一次性的 Lima Linux 客户端。默认循环覆盖 NAT、
DHCP/DNS、直连 HTTPS，以及通过 mihomo `mixed-port` 的显式 HTTPS。macOS
当前支持的透明代理路径是 TUN，因为当前 mihomo Darwin redir listener 不受支持。

CI 当前只运行快速单元测试门禁。对任何可能改变真实流量或主机网络状态的改动，
请在本地、夜间任务或手动 macOS 门禁中运行 `make lab-test`。

策略组控制面改动可以先运行一个更小的非 root 集成门禁：

```sh
make policy-control-test
```

这个 gate 会在 `runtime/integration/` 下写入 imported mihomo fixture，渲染
OpenSurge 的 gateway overlay，启动真实 mihomo 二进制，但不启动 dnsmasq、pf、
TUN，也不需要 sudo。它会用 live external-controller API 验证 `omg policies`、
`omg policy-select` 和 `omg connections`，也会验证 `omg snapshot` 可以在
mihomo 运行时聚合 status、doctor、leases、logs、policies 和 connections。
它会先验证未知 policy 会以机器可读 JSON 错误被拒绝，再在同一个 runtime 目录内
重启 mihomo，验证 `profile.store-selected` 能恢复选中的策略。它证明的是 mihomo
控制面契约，不证明全 LAN 路由或透明代理捕获。

透明代理门禁是 `make lab-test-tun`。它比默认 lab 路径更严格，因为客户端不使用
`mixed-port`；测试必须证明 mihomo 通过 TUN 观察到了客户端 HTTPS 连接。

真实设备测试仍然是单独的里程碑级检查，用来验证 Wi-Fi 行为、设备特有协议差异
和 IPv6。集成测试期间绝不要在普通家庭或办公室 LAN 上启用本项目的 DHCP 服务器。
隔离下游 LAN 的 smoke 计划见 `tests/real-device/README.zh-CN.md`。
