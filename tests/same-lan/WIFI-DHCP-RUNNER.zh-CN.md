# same-WiFi DHCP imported egress 全功能 runner

[English](WIFI-DHCP-RUNNER.md) | 简体中文

这个 runner 是高风险的真实 Wi-Fi 验收：Mac 与 Android 仍在同一 Wi-Fi，路由器
DHCP 已由操作者手动关闭，OpenSurge 在该 Wi-Fi 上接管 DHCP/DNS，并通过 mihomo
TUN 与 imported provider-backed `TunEgress` 完成策略切换验证。

它使用独立的 `gateway.mode: "same_wifi_dhcp"`。不要通过打开旧
`same_lan` 模式的 DHCP 来替代它：旧模式仍是 DHCP disabled 的窄范围 default-gateway
smoke。

## 前置条件

- 只在专门测试 SSID 上执行；路由器 DHCP 与 OpenSurge DHCP 绝不能同时运行。
- Mac 已在 Wi-Fi service 上配置手动 IPv4；当前 runner 会检查该地址仍是手动配置。
- 保留一台备用恢复设备，也使用不在 DHCP 池内的静态地址，并已验证能打开路由器管理页。
- 所有必须保留的静态地址都已列出。例如 Mac 是 `.20`、恢复设备是 `.21`、LAN
  proxy 是 `.101` 时，租约池可使用 `.120-.199`，不能包含前三者。
- 为避免受控本机 CONNECT proxy 的上游连接再次进入 TUN，准备一个上述保护地址上的
  HTTP proxy，例如 `192.168.1.101:8080`。这条 proxy 是受控 helper 的下一跳，不是
  Android 的显式 proxy。
- Android 测试设备设置为 DHCP/自动获取、显式 HTTP proxy 关闭，并已在 ADB 中显示为
  `device`。
- 已按 [same-WiFi DHCP 恢复参考](WIFI-DHCP-RECOVERY.zh-CN.md) 保存路由器管理入口、
  原 DHCP 配置和恢复步骤。

runner **不会**替你登录或关闭路由器 DHCP。启动时必须显式传入
`OMG_SAME_WIFI_DHCP_ROUTER_DHCP_DISABLED=confirmed`；这只是人工动作的安全收据，
不是对路由器状态的远程探测。

## 执行

在路由器 DHCP 已关闭、Mac 仍能通过静态地址打开路由器后，执行：

```sh
OMG_SAME_WIFI_DHCP_ROUTER_DHCP_DISABLED=confirmed \
OMG_SAME_WIFI_DHCP_PROTECTED_IPS=192.168.1.101 \
OMG_SAME_WIFI_DHCP_EGRESS_UPSTREAM_HTTP_PROXY=192.168.1.101:8080 \
make same-wifi-dhcp-start-imported-egress
```

默认租约池会根据 Mac 的 `/24` 生成 `.120-.199`。如需调整，显式提供：

```sh
OMG_SAME_WIFI_DHCP_RANGE_START=192.168.1.120 \
OMG_SAME_WIFI_DHCP_RANGE_END=192.168.1.199 \
... make same-wifi-dhcp-start-imported-egress
```

让 Android 忘记并重新加入测试 Wi-Fi，或切换 Wi-Fi 以触发 DHCP 续租；不要手动设置
它的网关或 DNS。确认它已获得池内地址后，运行：

```sh
make same-wifi-dhcp-adb-check-imported-egress
```

停止 OpenSurge 时，必须在重新打开路由器 DHCP 前执行：

```sh
make same-wifi-dhcp-stop
```

兼容的完整 target 名称也保留为 `same-lan-*-wifi-dhcp-*`，但新名称更准确地表达了
same-WiFi DHCP 接管的边界。

## runner 的证据

ADB gate 成功时会要求：

- Android IPv4 位于 OpenSurge DHCP 池；`omg leases --format json` 与
  `dnsmasq.log` 都有对应的 DHCPACK/租约；
- Android 默认路由经 Mac、DNS 查询命中 Mac 的 dnsmasq 日志，且没有全局显式 proxy；
- live provider 状态含 `tun-egress-provider` 与 `egress-proxy`，并成功执行一次
  `provider-update`；
- 无显式代理 HTTPS 首先命中 `TunEgress[DIRECT]`，没有受控 proxy CONNECT；
- `policy-select` 切到 `egress-proxy` 后，mihomo 日志命中
  `TunEgress[egress-proxy]`，受控 proxy 日志出现 `CONNECT <host>:443`；
- stop 后 runtime state、PF anchor、mihomo/dnsmasq listener 和本机 egress helper
  均被清理，IPv4 forwarding 回到启动前状态。

## 恢复与边界

`same-wifi-dhcp-stop` 只恢复 OpenSurge 修改的 host 状态，不会重新打开路由器 DHCP，
也不会把 Mac 改回 DHCP。随后应由操作者重新启用路由器 DHCP、让 Mac 与 Android 改回
自动获取，并至少验证一台客户端能重新自动获得地址并上网。

该 runner 证明一台 Android 的真实同 Wi-Fi DHCP、DNS、TUN、provider、policy switch
和受控本地 CONNECT egress 路径。它不证明全屋设备兼容、IPv6、DoH/Private Relay、
UDP/QUIC、完整订阅兼容性、真实远端节点或远端出口 IP。
