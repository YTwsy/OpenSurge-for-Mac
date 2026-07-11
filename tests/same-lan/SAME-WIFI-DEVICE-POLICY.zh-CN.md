# same-WiFi 双设备策略真机 gate

这个 gate 验证 `same_wifi_dhcp` 下两台真实 IPv4 客户端的精确 DHCP reservation、独立
selector、设备规则槽位、UDP fail-closed 与完整恢复。它仍是 **Experimental / cooperative
IPv4**：同一 Wi-Fi 上手工指定路由器网关或使用 IPv6 的客户端可以绕过 Mac。

开始前先完成 [恢复参考](WIFI-DHCP-RECOVERY.zh-CN.md)，准备一台池外静态恢复设备，并
记录路由器 DHCP 配置。两台测试设备必须关闭 MAC 随机化，记录该 SSID 实际使用的 Wi-Fi
MAC；启动 OpenSurge 前先断开它们，避免旧租约占用预留地址。Mac 使用池外固定 IPv4，
路由器 DHCP 由操作者手工关闭。

最小环境示例（地址必须按实际 LAN 调整）：

```sh
export OMG_SAME_WIFI_DHCP_ROUTER_DHCP_DISABLED=confirmed
export OMG_SAME_WIFI_DHCP_PROTECTED_IPS=192.168.1.21,192.168.1.101
export OMG_SAME_WIFI_DHCP_RANGE_START=192.168.1.120
export OMG_SAME_WIFI_DHCP_RANGE_END=192.168.1.199
export OMG_SAME_WIFI_DHCP_EGRESS_UPSTREAM_HTTP_PROXY=192.168.1.101:8080
export OMG_SAME_WIFI_DEVICE_ONE_MAC=aa:bb:cc:dd:ee:01
export OMG_SAME_WIFI_DEVICE_ONE_IP=192.168.1.121
export OMG_SAME_WIFI_DEVICE_ONE_ADB_SERIAL=serial-one
export OMG_SAME_WIFI_DEVICE_TWO_MAC=aa:bb:cc:dd:ee:02
export OMG_SAME_WIFI_DEVICE_TWO_IP=192.168.1.122
export OMG_SAME_WIFI_DEVICE_TWO_ADB_SERIAL=serial-two
```

受控 LAN HTTP proxy 必须使用池外静态地址，并列入 protected IP。runner 会先主动发送
DHCPDISCOVER；只要仍收到路由器或其他服务器的 OFFER 就拒绝启动。

运行：

```sh
make same-wifi-dhcp-start-device-policy
# 现在让两台测试设备重新加入 Wi-Fi，并确认 IP/DNS 均为自动
make same-wifi-dhcp-adb-check-device-policy
make same-wifi-dhcp-stop
```

ADB gate 要求并保存以下证据：

- 两台设备的 wlan0 MAC 与 reservation 完全一致，分别收到预定 IPv4；
- `omg leases`、DHCPACK、`omg devices` 的 `lease_match` 与
  `policy_identity_ready` 同时成立；
- mihomo 日志包含准确源 IPv4，并分别命中 `device/<id>/default`；
- 设备一的 `device/<id>/policy-test` 规则槽位可从 DIRECT 即时切到受控 proxy；
- 两个 default selector 反向切换后互不影响；
- HTTP-only 出口收到 UDP/443 时命中 REJECT，日志中不得出现同源 DIRECT；
- 手机没有显式代理，透明 HTTPS 由 TUN 捕获。

停止后先在路由器中恢复 DHCP，再把两台设备切回自动 IP/DNS，并运行恢复 gate：

```sh
export OMG_SAME_WIFI_DHCP_ROUTER_DHCP_RESTORED=confirmed
export OMG_SAME_WIFI_DHCP_CLIENTS_AUTOMATIC=confirmed
make same-wifi-dhcp-verify-device-policy-recovery
```

恢复 gate 会主动要求至少一个 DHCP OFFER，然后才把 Mac 恢复为 DHCP；随后检查 Mac 的
`server_identifier`、默认路由与 HTTPS，并检查两台设备已不再以 Mac 为默认网关、能够
通过恢复后的路由器访问 HTTPS。未实际完成这些命令和人工路由器步骤前，不得宣称
same-WiFi per-device 真机 gate 已通过。
