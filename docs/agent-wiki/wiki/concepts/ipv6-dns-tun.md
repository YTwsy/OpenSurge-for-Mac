# IPv6 DNS 与 TUN

当任务涉及 AAAA、fake IPv6、TUN IPv6、RA、SLAAC、IPv6 forwarding 或 IPv6
接管结论时，先读本页。

## 两个用户设置

```yaml
dns:
  ipv6: false

transparent:
  tun_ipv6: "off" # off, auto, always
```

它们彼此独立：

- `dns.ipv6` 只控制 mihomo DNS 是否回答 AAAA；它不建立 IPv6 路由。
- `transparent.tun_ipv6` 只控制 TUN IPv6。关闭 DNS IPv6 时，IPv6 字面地址仍可能
  进入 TUN，但普通域名不会从 OpenSurge DNS 获得 AAAA。

`auto` 会检查上游接口的非 link-local IPv6 地址与 IPv6 默认路由；两者都存在才生效。
`always` 绕过 mihomo 的宿主 IPv6 检查，主要供明确需要强制开启的网络和受控 Lab 使用。
状态输出同时提供 requested、effective 和 reason。

## 下游接管边界

当前 TUN IPv6 是宿主 Mac 的 VIF 能力，不是下游 IPv6 网关能力：

```text
Mac 本机 IPv6 socket
        |
fake AAAA / mihomo route
        |
mihomo VIF IPv6
        |
策略出口
```

OpenSurge 不发送 RA、不配置下游 ULA，也不改变 IPv6 forwarding。Virtual Lab
验证过本机 VIF 路径，同时发现 Darwin 不会把从下游 bridge 转发来的 IPv6 包交给
mihomo utun；RA、静态路由、PF `route-to` 和 NAT66 均未建立可用的数据面。因此
`isolated_lan`、`same_lan` 和 `same_wifi_dhcp` 都不能据此宣称接管下游 IPv6。

未来若实现下游 IPv6 接管，必须采用经证明的用户态数据面或其他 macOS 支持路径，
并另设真实设备验收。不能只因为 dnsmasq 能发送 RA 就把能力标记为完成。

## 验证

- 配置、渲染和生命周期逻辑：`make test`
- IPv4 网关回归：`make lab-test`
- 既有 TUN 回归：`make lab-test-tun`
- DNS AAAA 与本机 VIF IPv6：`make lab-test-tun-ipv6`

IPv6 gate 要求客户端能通过网关的 IPv4 DNS 获得 fake AAAA，本机 IPv6 socket
通过 mihomo VIF 完成 HTTPS，mihomo 日志和受控 CONNECT 出口都记录该请求。它还要
确认 dnsmasq 未启用 RA、IPv6 forwarding 前后不变、停止后 VIF 地址消失。该 gate
不证明下游设备 IPv6 数据面或真实设备兼容性。
