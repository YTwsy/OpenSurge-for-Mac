# Tailscale 远端子网与 Exit Node 真机验证

> 归档于 2026-10-05。历史执行记录已结束；保留原来的结果和未覆盖项。
> 来源：`docs/agent-wiki/sources/validation/tailscale-4via6-subnet-smoke.md`，文档快照 `405daacba9720731608dba46dd2c7592e62b40d5`；快照提交不冒充被测提交。
> 当前方法见 [验证门槛](../../sources/validation/test-gates.md)，当前适用性见 [证据入口](../../sources/validation/evidence-map.md)。以下保留原文，历史指令不作为新任务的默认指令。

本文记录 OpenSurge 经远端 Subnet Router 访问私网 SOCKS5 服务，以及使用
同一远端节点作为 Exit Node 的大致验证方法和最终验收状态。

## 大致方法

1. 在远端 LAN 部署可用的 SOCKS5 服务，确认远端 Subnet Router 能访问它，
   并完成路由发布、批准和访问授权。双方 IPv4 网段重叠时，可按官方
   [4via6 方案](https://tailscale.com/docs/features/subnet-routers/4via6-subnets)
   配置能够区分远端网络的访问目标。
2. 在 OpenSurge 中启用 Tailscale、接受远端路由，配置服务域名所属的 MagicDNS
   后缀，并授权本机和受测下游终端。涉及 IPv6 名称解析时允许 AAAA 查询；
   这不要求额外启用下游 IPv6 接管或发布 RA。
3. 让下游终端通过 OpenSurge 使用网关和 DNS。在终端自身执行 curl，可通过
   ADB 调用已有工具；先以远端 SOCKS5 服务作为显式代理访问可达的 HTTPS 网站。
   使用 `socks5h` 时由远端服务解析最终网站名称，代理服务名称仍由终端解析。
   使用 `--noproxy ''` 清空代理例外，避免跳过待测代理。
4. 以 SOCKS5 握手、TLS 证书验证和预期 HTTP 响应共同判定成功，并在本地日志中
   核对受测来源、远端服务目标和 `open-surge/tailscale` action。Mac 本机和下游
   终端分别验收；需要独立确认具体路由器时，在远端核对路由归属或对应转发报文。
5. 单独验证普通公网 Exit Node：将受测终端的默认出口选为目标 Exit Node，
   在不指定上述 SOCKS5 代理的情况下访问可达的 HTTPS 网站，核对实际出口选择、
   请求结果和日志。子网服务可达与普通公网出口可用应分别记录。

远端节点可以同时承担两种角色，但访问私网服务使用其子网路由能力，普通公网
请求使用默认出口能力；两项验收不能相互替代。角色区别见
[Tailscale subnet routers](https://tailscale.com/docs/features/subnet-routers)，
代理参数见 [curl 代理选项](https://curl.se/docs/manpage.html#--proxy)。

## 最终验收状态

本轮真机测试按功能可达性验收：每项曾成功一次即记为通过，后续网络波动不
撤销已经确认的成功结论。子网服务与普通公网 Exit Node 均已取得实际 HTTPS
请求成功结果，并核对了证书验证、网关出口选择和对应路径日志。

| 验证项 | 结果 |
| --- | --- |
| Mac 本机经托管 Tailscale 访问远端 SOCKS5 服务并完成 HTTPS 请求 | 已通过 |
| 下游终端经 OpenSurge 访问同一远端服务并完成 HTTPS 请求 | 已通过 |
| 下游终端使用同一远端节点作为普通公网 Exit Node | 已通过 |

本记录仅确认上述功能路径已取得成功结果；具体转发节点的独立抓包确认、
UDP/QUIC、国际互联网出口和长期稳定性不在本次已验证范围内。本记录不替代
`make lab-test-tailscale`，也不改变自动化测试与其他网络门槛的验收要求。
