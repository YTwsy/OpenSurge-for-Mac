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
- `make real-device-stop`：停止真实设备 smoke 配置并检查清理状态。

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
- Mac 侧 `curl --proxy http://192.168.50.1:17890 https://example.com/` 能通过
  mihomo `mixed-port` 访问 HTTPS。

这些信号证明真实设备拓扑、DHCP/DNS、gateway runtime、pf anchor、forwarding、
以及 Mac 侧显式代理探测已经工作。它们还不等于真实手机上的三段端到端验证都已
完成。

## 尚未宣称已通过的真实手机检查

以下检查需要在真实手机或等价物理客户端上继续执行，完成后才能宣称对应路径已
通过：

- 无代理直连 HTTPS/NAT：手机 Wi-Fi HTTP proxy 关闭，打开 HTTPS 页面成功；
  Mac 侧同时能看到该手机租约和 DNS 查询。
- 显式 HTTP 代理 HTTPS：手机 Wi-Fi HTTP proxy 设为 `192.168.50.1:17890`，
  打开 HTTPS 页面成功；如果日志级别允许，`mihomo.log` 应出现来自该手机 IP 的
  HTTPS 连接。
- 真实设备 TUN 透明模式：运行 `make real-device-start-tun`，手机保持无显式
  代理，打开 HTTPS 页面成功，并在 `mihomo.log` 中看到来自该手机 IP 的目标
  连接。

记录验证进度时，优先保存 artifact 和高层结论。不要把一次性完整日志、私有
设备标识、sudo 密码或家庭网络细节写入 wiki。
