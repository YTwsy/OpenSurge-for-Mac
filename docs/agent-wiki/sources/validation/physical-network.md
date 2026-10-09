# 物理客户端与网络恢复验收

本页维护独立下游 LAN、同 LAN 旁路、DHCP 接管和物理断链的可复用方法与通过标准。
自动化入口以 [real-device runner](../../../../tests/real-device/smoke.sh)、
[same-LAN runner](../../../../tests/same-lan/smoke.sh) 和 [恢复手册](../../../../tests/same-lan/WIFI-DHCP-RECOVERY.zh-CN.md) 为准；
它们不能由主机上的连通性探测替代。历史结果经 [证据入口](evidence-map.md#物理网络) 定向读取，不在本页追加日期报告。

## 独立下游 LAN

Mac 上游连接 Internet，下游使用独立 Ethernet；备用路由器/AP 只工作在 bridge/AP 模式，
不运行第二套 DHCP、NAT 或路由。默认测试 LAN 为 `192.168.50.1/24`，客户端租约范围
`192.168.50.100-200`，router/DNS 均指向 Mac。Lab 使用 `/22`，两者不能在不同接口留下
相同网关地址；冲突排查见 [Lab 环境](lab-gates.md#地址与证据隔离)。

| 检查路径 | 入口 | 需要的证据 |
| --- | --- | --- |
| NAT 与显式代理 | `make real-device-start-off` | 客户端无代理的 HTTPS/NAT 与显式 mixed-port 请求分别成功，租约、DNS 源 IP、代理连接可对应 |
| TUN 透明接入 | `make real-device-start-tun` | 客户端无显式代理，DNS 返回 fake IP，真实客户端目标连接出现在 Mihomo TUN 日志 |
| 最小受控代理出口 | `make real-device-start-tun-proxy` | 目标域名命中 `open-surge-egress`，受控代理出现对应 CONNECT |
| 观察与收尾 | `make real-device-client-check` / `make real-device-status` / `make real-device-stop` | 运行时服务证据齐全；停止后清除 state、PF、服务与测试 LAN IP，恢复原 forwarding |

runner 在同一个终端流程中使用 sudo；密码不进入仓库、文档、脚本或命令行。
客户端证据必须来自真实终端，不能只检查 Mac 能否 `dig`。记录客户端是否使用显式代理，
并把租约、DNS、目标请求与 Mihomo 日志对应起来。

默认配置的 `MATCH,DIRECT` 不证明订阅或远端出口。受控 `upstream_proxy` 只覆盖一个上游和
匹配域名；本机默认受控代理为 `127.0.0.1:18080`。要证明独立远端出口，还需客户端出口 IP、
代理端记录与 Mihomo action 交叉印证；这些结果仍不替代确定性的 Lab 门槛。

## 同 LAN 旁路

使用 `gateway.mode: same_lan`、同一个 LAN/上游接口、`dhcp.enabled: false` 和 TUN。
Mac LAN IP 上的 dnsmasq 为 DNS-only，向 `127.0.0.1#1053` 转发，PF NAT 排除本地 LAN。
测试终端手工把网关和 DNS 指向 Mac；本门槛不在主 LAN 发布 DHCP。

```sh
make same-lan-start-tun
make same-lan-adb-check
make same-lan-status
make same-lan-stop
```

ADB 检查授权设备、IPv4 默认路由 `via <mac-lan-ip>`、经 Mac 的 DNS、无显式代理请求，
再核对 Mac 侧 DNS 源 IP 与 Mihomo TUN 连接。缺少 `curl`、`wget`、`nc` 或 DNS 工具时，
明确记录客户端探针缺口；人工浏览器观察不能写成 ADB 自动化通过。停止检查 runtime、PF、
forwarding 和 listener 恢复。仅此路径不证明 IPv6、DoH/Private Relay、UDP/QUIC 或全屋部署。

## 受控出口与 imported provider 切换

`make same-lan-start-tun-proxy` 使用最小 `upstream_proxy`。配置入口为
`OMG_SAME_LAN_UPSTREAM_PROXY_ENABLED`、`TYPE`、`SERVER`、`PORT`、`MATCH_DOMAIN`
（后四项同样以 `OMG_SAME_LAN_UPSTREAM_PROXY_` 为前缀）。支持 HTTP/SOCKS5，
以 `api.ipify.org` 等目标的客户端出口、Mac DNS 日志和匹配规则共同证明路径。
只有一个 group 成员时不能据此证明策略切换。

```sh
make same-lan-start-tun-imported-egress
make same-lan-adb-check-imported-egress
make same-lan-stop
```

该 fixture 来自 `tests/lab/mihomo-profile.imported-tun-egress.yaml`，为 `TunEgress`
提供 `DIRECT` 与 provider 的 `egress-proxy`。先观察 DIRECT 请求且受控代理无命中，
再切换 selector，要求新请求命中 `egress-proxy` 并出现对应 CONNECT。运行中还需验证
provider 读取与更新；完整断言由 runner 维护。这证明受控 provider 的 TUN 出口切换，
不证明真实订阅节点或远端出口 IP。

生成的 `mihomo.profile` 相对 `runtime/same-lan/config-tun.yaml` 为
`./mihomo-profile.imported-tun-egress.yaml`。helper 必须同时就绪 HTTP provider 与 CONNECT
端口；受控代理的上游 DNS/socket 绑定真实上游，避免重新进入待测 TUN。

## 局域网 DHCP 接管

高风险路径使用独立的 `same_wifi_dhcp`，不改变 `same_lan` 禁用 DHCP 的约束。
先阅读恢复手册，保留 Mac 静态 IPv4，并在手工关闭路由器 DHCP 后设置
`OMG_SAME_WIFI_DHCP_ROUTER_DHCP_DISABLED=confirmed` 与非空的
`OMG_SAME_WIFI_DHCP_PROTECTED_IPS`。前者是人工确认，不冒充主动探测。
runner 默认池为该 `/24` 的 `.120-.199`，不得包含 Mac 或受保护地址。

```sh
make same-wifi-dhcp-start-imported-egress
make same-wifi-dhcp-adb-check-imported-egress
make same-wifi-dhcp-stop
```

通过要求真实终端位于地址池且对应 `omg leases` 与 DHCPACK、DNS 源 IP、无显式代理，
并完成上节的 DIRECT/provider 切换。stop 仅清理网关，不自动恢复路由器和客户端 DHCP。
完整恢复还必须证明路由器 DHCP 已恢复、Mac 回到 DHCP、真实客户端重新自动获取并上网。

双设备策略验收使用以下入口；客户端 SSID Wi-Fi MAC、固定 reservation 与 ADB serial
必须一一对应，启动前主动检查不存在其他 DHCP OFFER：

```sh
make same-wifi-dhcp-start-device-policy
make same-wifi-dhcp-adb-check-device-policy
make same-wifi-dhcp-stop
make same-wifi-dhcp-verify-device-policy-recovery
```

通过要求两设备身份、独立 default/rule selector、准确源 IP 和 UDP REJECT；恢复阶段要求
路由器 OFFER、Mac DHCP `server_identifier`/默认路由及两客户端 HTTPS。
`client_validation_skipped` 和 `complete_static` 是明确的流程选择，不能替代这些证据。

## 物理上游断链恢复

影响 Mihomo 恢复、共用 Wi-Fi 的拓扑或接管清理时，额外检查实际断链和重新关联：

1. 断链前分别验证 DIRECT 与真实/受控代理目标，记录 rule、chain 和日志。
2. 人工断开并重新关联 Wi-Fi，确认接口、静态 IPv4、router、DNS 恢复。
3. 若 Mihomo 路径持续失败，保存 Wi-Fi 时间线与日志，再执行 `sudo omg restart-mihomo --config <path>`。
4. 确认只有 Mihomo PID 改变；dnsmasq PID、PF、forwarding、静态网络和 DHCP 恢复阶段保持一致。
5. 复验 DIRECT 与代理目标并确认旧日志已归档；最后完成 stop 及路由器、Mac、客户端 DHCP 恢复。

状态事务单测和虚拟 LAN 不证明物理 Wi-Fi 恢复、睡眠/唤醒或路由器重启；需要相应实机记录。
