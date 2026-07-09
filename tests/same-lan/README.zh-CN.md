# same-LAN TUN smoke 测试

简体中文 | [English](README.md)

这份指南覆盖同一家庭或办公室 LAN 内的 TUN 默认网关 smoke。它不同于
`tests/real-device/` 的隔离下游 LAN：Mac 和 Android 测试设备都连接同一个主
LAN，OpenSurge 不接管主 LAN DHCP。

## 拓扑

```text
Home router / main Wi-Fi: 192.168.1.1
        |
        +-- Mac en0: 192.168.1.20
        |     OpenSurge same_lan + mihomo TUN + DNS-only dnsmasq
        |
        +-- Android phone: 192.168.1.x
              default gateway: 192.168.1.20
              DNS: 192.168.1.20
```

第一轮验收只面向一台测试手机。不要在主 LAN 上启动 OpenSurge DHCP，也不要把主
路由器的 DHCP 全局下发改成 Mac，除非你已经准备好影响所有设备。

## 启动

默认 runner 会从 macOS 默认路由推断接口，并从该接口读取 Mac IPv4 地址：

```sh
make same-lan-start-tun
```

也可以显式指定：

```sh
OMG_SAME_LAN_IFACE=en0 \
OMG_SAME_LAN_MAC_IP=192.168.1.20 \
make same-lan-start-tun
```

runner 会生成 `runtime/same-lan/config-tun.yaml`，其中：

- `gateway.mode: "same_lan"`；
- `gateway.interface` 和 `gateway.upstream_interface` 相同；
- `dhcp.enabled: false`；
- `dns.listen` 绑定 Mac 的主 LAN IP；
- `dns.upstream: "127.0.0.1#1053"`，让客户端 DNS 进入 mihomo fake-ip；
- `transparent.mode: "tun"`。

## Android ADB 验证

先让测试 Android 手机的 Wi-Fi 默认网关和 DNS 指向 Mac 的 LAN IP。当前 runner
不尝试永久修改手机 Wi-Fi 配置；它通过 ADB 自动采集验证结果，避免依赖人工口头
回报。

```sh
make same-lan-adb-check
```

多台设备连接时指定序列号：

```sh
OMG_SAME_LAN_ADB_SERIAL=57081FDCQ008KZ make same-lan-adb-check
```

ADB 检查会验证：

- 设备处于 authorized `device` 状态；
- Android 默认路由包含 `via <mac-lan-ip>`；
- Android 镜像带有 `nslookup` 或 `dig` 时，能查询
  `@<mac-lan-ip> example.com`；
- Android 全局显式代理没有被设置为测试成功的前提；
- Android 通过 `curl`、`wget` 或 `nc` 发起到 `https://example.com/` 或
  `example.com:443` 的无显式代理探针；
- Mac 侧 `dnsmasq.log` 和 `mihomo.log` 能看到对应 DNS/fake-ip 与 TUN 连接。

如果 Android 镜像没有 `nslookup` 和 `dig`，ADB gate 会继续执行 TCP 探针，并
通过 Mac 侧 `dnsmasq.log` 中 Android 源 IP 的查询记录推断 DNS 路径。如果镜像也
没有 `curl`、`wget` 和 `nc`，这个 gate 才会停在缺少客户端探针工具的位置。后续
可以只补一个极薄 probe APK，不要把网关事实判断搬进 APK。

## 代理出口 smoke

第一轮代理出口验证不需要导入完整订阅，先用生成的最小 mihomo 规则更稳。把
`upstream_proxy` 指向一个已知可用的 LAN 代理，并只匹配一个诊断域名：

```sh
OMG_SAME_LAN_TEST_HOST=api.ipify.org \
OMG_SAME_LAN_UPSTREAM_PROXY_NAME=same-lan-http-egress \
OMG_SAME_LAN_UPSTREAM_PROXY_TYPE=http \
OMG_SAME_LAN_UPSTREAM_PROXY_SERVER=192.168.1.101 \
OMG_SAME_LAN_UPSTREAM_PROXY_PORT=8080 \
OMG_SAME_LAN_UPSTREAM_PROXY_MATCH_DOMAIN=api.ipify.org \
make same-lan-start-tun-proxy

OMG_SAME_LAN_TEST_HOST=api.ipify.org make same-lan-adb-check
```

如果要测 LAN SOCKS5 代理，把 `OMG_SAME_LAN_UPSTREAM_PROXY_TYPE` 改成
`socks5`，并指定对应 server/port。生成的 mihomo 配置会包含一个
`open-surge-egress` select group 和一条规则：

```yaml
rules:
  - DOMAIN,api.ipify.org,open-surge-egress
  - MATCH,DIRECT
```

只有以下证据同时成立，才宣称真实代理出口被验证：

- Android 默认网关和 DNS 仍指向 Mac LAN IP；
- Android 全局显式代理仍未设置；
- `dnsmasq.log` 看到 Android 源 IP 查询 `api.ipify.org`；
- `mihomo.log` 看到 `Domain(api.ipify.org) using open-surge-egress[...]`；
- Android 浏览器打开 `https://api.ipify.org/` 时显示预期的上游代理出口 IP。

2026-07-09 的 same-LAN run 已经用 Pixel 测试手机、`api.ipify.org` 和一个 LAN
HTTP 代理验证了这条路径。Android 页面和 Mac 侧直连该代理检查得到同一个出口 IP，
同时 `mihomo.log` 显示命中 `open-surge-egress[same-lan-http-egress]`。

策略组切换属于下一层 smoke。当前自动生成的 group 只有一个代理成员，所以可以证明
“命中代理”，但不能证明有意义的切换。要验证切换，至少让
`open-surge-egress` 同时包含 LAN 代理和 `DIRECT` 两个候选，然后用
`omg policy-select --config <path> --group open-surge-egress --policy <member>`
切换当前选中项，再重复 `api.ipify.org` 探针。

## 停止

```sh
make same-lan-stop
```

预期停止后：

- OpenSurge runtime state 被移除；
- same-LAN PF anchor 被卸载；
- IPv4 forwarding 恢复到启动前状态；
- DNS listener 不再占用 Mac LAN IP 的 53 端口；
- Mac 和主 LAN 恢复到启动前的普通网络状态。

## 当前边界

这个 smoke 只证明一台测试 Android 在同一 LAN 内把默认网关/DNS 指向 Mac 后，
无显式代理流量可以进入 OpenSurge TUN 路径。配合 `same-lan-start-tun-proxy` 时，
它也可以证明单个域名的真实上游代理出口。它不证明主路由 DHCP 全局下发、所有设备
兼容、IPv6、DoH/Private Relay、UDP/QUIC、订阅导入或策略组切换。
