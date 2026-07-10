# same-WiFi DHCP 恢复参考

[English](WIFI-DHCP-RECOVERY.md) | 简体中文

这份 runbook 面向一种高风险但接近 Surge 使用方式的测试：Mac 和下游设备连接同一个
主 Wi-Fi，路由器 DHCP 被关闭，OpenSurge 后续在同一 LAN 上为客户端提供网关/DNS。

当前自动化 same-LAN smoke 仍然默认 `dhcp.enabled: false`，不会接管主 LAN DHCP。
只有在专门测试 Wi-Fi、明确知道如何进路由器管理页、并准备好恢复手段时，才手动尝试
关闭主路由 DHCP。

## 测试前记录

先保存这些信息，最好截图或写在离线笔记里：

- Wi-Fi SSID 和密码；
- 路由器管理地址，例如 `192.168.1.1`；
- 路由器管理员账号和密码；
- 路由器当前 LAN IP、子网掩码、DHCP 开关、DHCP 地址池；
- Mac 当前 Wi-Fi 接口名，通常是 `en0`；
- Mac 当前 IPv4 地址、路由器地址和 DNS；
- 至少一个备用客户端的静态 IPv4 配置。

查看 Mac Wi-Fi 信息：

```sh
networksetup -listallhardwareports
ipconfig getifaddr en0
route -n get default
scutil --dns | sed -n '1,80p'
```

## 建议的恢复拓扑

保留两个可进入路由器管理页的入口：

- Mac 使用静态 IPv4，例如 `192.168.1.20/24`，router 为 `192.168.1.1`；
- 另一台手机或笔记本也准备静态 IPv4，例如 `192.168.1.21/24`，router 为
  `192.168.1.1`。

这样即使路由器 DHCP 已关闭、OpenSurge 没有成功启动 DHCP，你仍然能访问
`http://192.168.1.1/` 重新打开路由器 DHCP。

## Mac 设为静态 IPv4

如果主 Wi-Fi 的网段是 `192.168.1.0/24`，路由器是 `192.168.1.1`：

```sh
sudo networksetup -setmanual "Wi-Fi" 192.168.1.20 255.255.255.0 192.168.1.1
sudo networksetup -setdnsservers "Wi-Fi" 192.168.1.1 1.1.1.1
```

确认仍能访问路由器：

```sh
ping -c 3 192.168.1.1
open http://192.168.1.1/
```

如果你的 Wi-Fi service 名不是 `Wi-Fi`，用 `networksetup -listallnetworkservices`
查实际名称。

## 关闭路由器 DHCP 后立刻检查

关闭路由器 DHCP 后，不要马上改更多设置。先确认：

```sh
ping -c 3 192.168.1.1
curl --max-time 3 http://192.168.1.1/ >/dev/null || true
```

如果 Mac 已经无法访问路由器，先恢复路由器 DHCP，不要继续启动 OpenSurge。

## 测试中失联时

按这个顺序恢复，尽量不要直接重置路由器：

1. 保持 Mac 连在原 Wi-Fi。
2. 把 Mac 设回同网段静态地址：

   ```sh
   sudo networksetup -setmanual "Wi-Fi" 192.168.1.20 255.255.255.0 192.168.1.1
   sudo networksetup -setdnsservers "Wi-Fi" 192.168.1.1 1.1.1.1
   ```

3. 打开 `http://192.168.1.1/`，重新启用路由器 DHCP。
4. 如果 Mac 不能进管理页，用备用手机或笔记本设静态 IP 后访问路由器。
5. 如果仍然无法访问路由器，使用网线直连路由器 LAN 口再设静态 IP。
6. 只有在忘记管理地址、密码或路由器配置不可恢复时，才使用硬件 reset。

## 测试完成后恢复

先停止 OpenSurge，再恢复主路由 DHCP：

```sh
make same-wifi-dhcp-stop
```

在路由器管理页重新启用 DHCP，并确认地址池恢复为测试前记录的范围。

然后把 Mac 的 Wi-Fi 改回 DHCP：

```sh
sudo networksetup -setdhcp "Wi-Fi"
sudo networksetup -setdnsservers "Wi-Fi" Empty
sudo ipconfig set en0 DHCP
```

重新连接 Wi-Fi：

```sh
networksetup -setairportpower en0 off
sleep 2
networksetup -setairportpower en0 on
```

确认恢复：

```sh
ipconfig getifaddr en0
route -n get default
scutil --dns | sed -n '1,80p'
ping -c 3 192.168.1.1
curl --fail --silent --show-error --max-time 5 https://example.com/ >/dev/null
```

## 客户端恢复

手机或其他测试设备如果被手动设过静态 IP、网关或 DNS，测试后改回自动获取：

- iOS/iPadOS：Wi-Fi 详情 -> 配置 IP -> 自动；配置 DNS -> 自动。
- Android：Wi-Fi 详情 -> IP 设置 -> DHCP；代理 -> 无。
- macOS 客户端：`sudo networksetup -setdhcp "Wi-Fi"`。

如果客户端仍拿不到地址，先忘记该 Wi-Fi 后重新加入。

## 安全边界

- 不要在日常主 Wi-Fi 上第一次测试这个模式；使用专门测试 SSID 更稳。
- 不要同时让路由器 DHCP 和 OpenSurge DHCP 在同一 LAN 发放地址。
- 不要把 OpenSurge 控制面裸露到 LAN；远程控制需要 token/auth。
- 任何“全屋可用”的结论都必须包含恢复验证：OpenSurge stop、路由器 DHCP 恢复、
  Mac 回到 DHCP、至少一台客户端重新自动获取地址并正常上网。
