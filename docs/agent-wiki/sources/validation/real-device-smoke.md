---
title: Real-device smoke
kind: source
status: seed
---

# 真实设备 smoke

真实设备 smoke 是 virtual LAN lab 之后的物理设备检查层。它用于确认下游设备
接入由 Mac 服务的隔离 LAN 后，能获得 OpenSurge for Mac 提供的 DHCP/DNS，
并通过 Mac 的 gateway 路径出站。

它不替代 `make lab-test` 或 `make lab-test-tun`。lab gates 仍然是可复现的
代码变更门槛；真实设备 smoke 记录的是当前机器、当前物理拓扑和真实客户端的
集成状态。

## 拓扑

稳定拓扑是：

- Mac 上游接口连接 Internet，例如 Wi-Fi `en0`。
- Mac 下游接口是独立 USB Ethernet，例如 `en7`，地址为 `192.168.50.1/24`。
- 下游 AP 或备用路由器只工作在 bridge/AP 模式，不运行 DHCP、NAT、防火墙或
  路由模式。
- 真实客户端应获得 `192.168.50.100-200` 范围内地址，router 和 DNS 都应为
  `192.168.50.1`。

## Runner

真实设备 smoke 的入口由 `tests/real-device/smoke.sh` 和 Makefile 目标提供：

- `make real-device-start-off`：启动显式代理模式，`transparent.mode: "off"`。
- `make real-device-start-tun`：启动 TUN 透明模式，`transparent.mode: "tun"`。
- `make real-device-client-check`：查看 status、leases、dnsmasq log 和
  mihomo log。
- `make real-device-stop`：停止真实设备 smoke 配置、释放下游测试 LAN IP 并检查
  清理状态。

runner 会在终端里触发一次 `sudo` 提示，并把 root-required 步骤收敛到同一条
流程里。不要把它描述成免密 root helper，也不要把 sudo 密码写入仓库、文档、
脚本或命令行。

## 当前验证进度

截至 2026-07-06 CST，本轮真实设备 smoke 已经验证：

- USB-C/Thunderbolt Ethernet 下游接口和备用 AP/bridge 拓扑可用。
- `make real-device-start-off` 能启动 explicit/off 配置。
- status 显示 gateway running，DHCP running，mihomo running，pf anchor loaded，
  IPv4 forwarding enabled。
- 真实物理客户端能获得 `192.168.50.100-200` 范围租约，router/DNS 为
  `192.168.50.1`。
- Mac 侧 `dig @192.168.50.1 example.com A` 能从 OpenSurge DNS 路径获得响应。
- 真实 Pixel 手机在 HTTP proxy 关闭时能打开 `https://example.com/`，Mac 侧
  `leases` 和 `dnsmasq.log` 能对应到该客户端租约与 `example.com` DNS 查询。
- 真实 Pixel 手机把 HTTP proxy 设为 `192.168.50.1:17890` 后能打开
  `https://example.com/`，`mihomo.log` 能看到来自该客户端的
  `example.com:443` 连接。
- `make real-device-start-tun` 能启动 TUN 配置，dnsmasq 上游切到
  `127.0.0.1#1053`，`example.com` 返回 `198.18.0.x` fake-ip。
- 真实 Pixel 手机在 TUN 模式下保持 HTTP proxy 关闭时能打开
  `https://example.com/`，`dnsmasq.log` 能看到该客户端的 `example.com`
  fake-ip 查询，`mihomo.log` 能看到该客户端到 `example.com:443` 的连接。
- `make real-device-start-tun-proxy` 能启动带最小 `upstream_proxy` 的 TUN 配置。
  本轮使用 Mac 本机受控 HTTP CONNECT proxy `127.0.0.1:18080`，手机保持 HTTP
  proxy 关闭后打开 `https://example.com/` 成功；`dnsmasq.log` 显示该客户端把
  `example.com` 解析为 `198.18.0.4`，`mihomo.log` 显示
  `example.com:443 match Domain(example.com) using
  open-surge-egress[real-device-egress]`，本机受控代理日志显示对应
  `CONNECT example.com:443`。
- `make real-device-stop` 能停止 gateway，清理 runtime state，停止 DHCP 和
  mihomo，卸载 real-device PF anchor，释放下游接口上的 `192.168.50.1` 测试
  LAN IP，并把 IPv4 forwarding 恢复为 disabled。

这些信号证明真实设备拓扑、DHCP/DNS、gateway runtime、pf anchor、forwarding、
无代理 NAT、显式 `mixed-port` 代理入口，以及 TUN 透明路径已经完成一次物理手机
smoke。它们也证明最小受控 upstream proxy 切片可以把指定域名从
`MATCH,DIRECT` 改为命中 `open-surge-egress`，并把请求交给受控代理。

## 仍未覆盖的范围

默认生成的 mihomo 配置仍是 `MATCH,DIRECT`。已验证的 proxy egress smoke 只覆盖
最小本机受控 upstream proxy 切片，不证明订阅规则、远端代理节点、策略组切换、
出口 IP 变化或真实分流策略生效。

`upstream_proxy` 只表达一个受控 HTTP/SOCKS5 上游和一个匹配域名。如果代理跑在
本机，runner 默认值会指向 `127.0.0.1:18080`。要宣称远端代理或真实出口 IP
验证，仍需要换成具有独立出口的实际代理，并用出口 IP、代理服务日志和
`mihomo.log` 交叉确认。

真实设备结果仍然是当前机器、当前物理拓扑、当前客户端的集成状态，不替代
`make lab-test` / `make lab-test-tun` 作为代码改动的可复现 lab 门槛。

记录验证进度时，优先保存 artifact 和高层结论。不要把一次性完整日志、私有
设备标识、sudo 密码或家庭网络细节写入 wiki。
