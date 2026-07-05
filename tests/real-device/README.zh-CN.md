# 真实设备隔离 LAN smoke 测试

简体中文 | [English](README.md)

这份指南用于虚拟 LAN lab 通过后的第一个真实设备里程碑。目标是在保持测试网络
与家庭或办公室 LAN 隔离的前提下，用物理客户端验证同一套 macOS 网关实现。

## 核心拓扑

是的：核心做法就是给 Mac 一个专用的下游 LAN 接口。测试设备只接入这个下游 LAN，
而 Mac 的另一个接口继续作为上游 Internet 路径。

```text
Home router / main Wi-Fi
        ^
        |
   Mac Wi-Fi en0
        |
   omg + mihomo + dnsmasq + pf
        |
   Mac USB Ethernet en7: 192.168.50.1
        v
   Test switch / spare router in AP or bridge mode
        v
   iPhone / PS5 / Switch / test laptop
```

在这个拓扑里，家里主路由的 DHCP 可以继续开启，因为它位于另一个广播域。本项目的
dnsmasq 实例会配置为只绑定下游接口，所以它的 DHCP 广播应留在 `en7`，不会出现在
主 Wi-Fi 上。

绝不要在家庭或办公室主 LAN 上运行本项目的 DHCP 服务器。

## 硬件要求

- 一台 Mac，带一个上游接口，通常是 Wi-Fi，例如 `en0`。
- 一个独立下游接口，通常是 USB 网卡，例如 `en7`。
- 一个测试交换机，或配置成 AP/bridge 模式的备用路由器。
- 一台测试笔记本，以及 iPhone、PS5、Switch 等一个或多个设备。

备用路由器在此测试中不能运行 DHCP、NAT、防火墙或路由模式。它只应该把
Wi-Fi/Ethernet 客户端桥接到 Mac 的下游 LAN。

## 预检查

先运行虚拟 LAN 门禁：

```sh
make lab-up
sudo -v
make lab-test
make lab-test-tun
make lab-down
```

识别 Mac 接口：

```sh
networksetup -listallhardwareports
route -n get default
ifconfig en7
```

预期：

- `upstream_interface` 是能访问 Internet 的接口，例如 `en0`。
- `gateway.interface` 是下游测试 LAN，例如 `en7`。
- 两个接口必须不同。
- 上游网络不能已经使用 `192.168.50.0/24`。

推荐用 smoke runner 自动完成本地配置、构建、下游地址绑定、root doctor、
启动和基础探测：

```sh
make real-device-start-off
make real-device-status
```

该 runner 会在终端里触发一次 `sudo` 提示，并把后续 root 步骤放在同一个
sudo 会话中执行，避免在多条命令之间反复刷新 sudo ticket。它不会安装免密
sudoers 规则，也不会给仓库里的可写二进制授予免密 root 权限。

如果下游接口不是 `en7`，用环境变量覆盖：

```sh
OMG_REAL_DEVICE_IFACE=en8 make real-device-start-off
```

手动流程仍然可以使用：

```sh
sudo ifconfig en7 inet 192.168.50.1 netmask 255.255.255.0 up
```

## 配置

在 `runtime/real-device/` 下创建本地配置。如果这些文件包含机器特定接口名或代理
设置，请不要提交。下面的示例假设命令从仓库根目录运行。

先使用显式代理模式：

```yaml
gateway:
  interface: "en7"
  lan_ip: "192.168.50.1"
  upstream_interface: "en0"

dhcp:
  binary: "./runtime/tools/bin/dnsmasq"
  enabled: true
  range_start: "192.168.50.100"
  range_end: "192.168.50.200"
  lease_time: "30m"
  domain: "realtest"

dns:
  listen: "192.168.50.1"
  port: 53
  upstream: ""

mihomo:
  binary: "./runtime/tools/bin/mihomo"
  config: "./runtime/real-device/mihomo.yaml"
  mixed_port: 17890
  redir_port: 0
  api_addr: "127.0.0.1:19090"
  secret: ""

pf:
  anchor_name: "com.apple/open_mihomo_gateway_real_device"
  redirect_tcp_to: 0

transparent:
  mode: "off"
  tun_device: "utun123"
  tun_stack: "mixed"
  tun_auto_route: true
  tun_auto_detect_interface: false
  tun_strict_route: false

runtime:
  dir: "./runtime/real-device"
```

透明 TUN 模式可以复制该配置，只改以下字段：

```yaml
dns:
  listen: "192.168.50.1"
  port: 53
  upstream: "127.0.0.1#1053"

transparent:
  mode: "tun"
```

当前生成的 mihomo 配置使用 `MATCH,DIRECT`。这个里程碑证明真实设备流量能被 Mac
网关捕获并转发；它还不证明订阅规则或远程代理出口行为。

## 显式代理 smoke

推荐启动方式：

```sh
make real-device-start-off
```

构建、写入 `runtime/real-device/config-off.yaml`、绑定下游地址、root doctor、
启动和基础 DNS/API/listener 探测都会由 runner 完成。

手动构建并启动：

```sh
GOCACHE=/private/tmp/omg-go-cache go build -o bin/omg ./cmd/omg
sudo ./bin/omg doctor --config runtime/real-device/config-off.yaml
sudo ./bin/omg start --config runtime/real-device/config-off.yaml
./bin/omg status --config runtime/real-device/config-off.yaml
./bin/omg leases --config runtime/real-device/config-off.yaml
```

让测试笔记本和设备连接下游 AP。笔记本应该获得类似 `192.168.50.100` 的地址，
router 和 DNS 都应为 `192.168.50.1`。

在测试笔记本上：

```sh
route -n get default
dig @192.168.50.1 example.com A
curl --noproxy '*' --fail --show-error https://example.com/
curl --proxy http://192.168.50.1:17890 --fail --show-error https://example.com/
```

在 iPhone、PS5、Switch 或其他设备上：

- 确认设备地址是 `192.168.50.x`。
- 如果设备显示这些细节，确认 router/DNS 是 `192.168.50.1`。
- 做显式代理测试时，把 HTTP proxy 设置为 `192.168.50.1:17890`。
- 打开一个简单 HTTPS 页面，或执行普通连通性检查。

停止并验证清理：

```sh
make real-device-stop
```

或手动：

```sh
sudo ./bin/omg stop --config runtime/real-device/config-off.yaml
./bin/omg status --config runtime/real-device/config-off.yaml
sysctl -n net.inet.ip.forwarding
sudo pfctl -s Anchors
```

## 透明 TUN smoke

推荐启动 TUN 配置：

```sh
make real-device-start-tun
```

或手动：

```sh
sudo ./bin/omg start --config runtime/real-device/config-tun.yaml
./bin/omg status --config runtime/real-device/config-tun.yaml
./bin/omg leases --config runtime/real-device/config-tun.yaml
```

客户端不要设置显式代理。从测试笔记本运行：

```sh
dig @192.168.50.1 example.com A
curl --noproxy '*' --fail --show-error https://example.com/
```

在 Mac 上确认 mihomo 观察到了客户端流量：

```sh
make real-device-client-check
tail -n 120 runtime/real-device/logs/mihomo.log
```

预期：日志中包含来自真实客户端地址的 TCP 连接，例如 `192.168.50.100` 到
`example.com:443`，或 smoke 测试中使用的目标。

停止并验证清理：

```sh
make real-device-stop
```

或手动：

```sh
sudo ./bin/omg stop --config runtime/real-device/config-tun.yaml
./bin/omg status --config runtime/real-device/config-tun.yaml
sysctl -n net.inet.ip.forwarding
sudo pfctl -s Anchors
```

## 验收清单

- 家庭或办公室主网络不受影响。
- DHCP 租约只发给下游测试 LAN 上的设备。
- 测试笔记本获得 `192.168.50.x`，router 为 `192.168.50.1`，DNS 为
  `192.168.50.1`。
- 显式代理模式下，直连 HTTPS 能通过 NAT 工作。
- 通过 `192.168.50.1:17890` 的显式 HTTPS 能工作。
- TUN 模式在客户端无代理设置时能工作。
- `mihomo.log` 在 TUN 模式下显示真实客户端流量。
- `stop` 会移除运行时状态并卸载 pf anchor。
- `stop` 后 IP forwarding 恢复到启动前的值。

## Artifact 清单

每次运行创建一个 artifact 目录：

```sh
mkdir -p artifacts/real-device/$(date +%Y%m%d-%H%M%S)
```

保存：

- `config-off.yaml` 和 `config-tun.yaml`。
- `host-before.txt`：`route -n get default`、下游 `ifconfig`、pf anchors、
  以及 `sysctl -n net.inet.ip.forwarding`。
- `doctor-off.txt` 和 `doctor-tun.txt`。
- `start-off.log` 和 `start-tun.log`。
- `status-during.txt` 和 `leases.txt`。
- `mihomo.log`。
- `client-laptop.txt`：route、DNS、curl 结果。
- `client-devices.md`：设备型号、IP、router/DNS、显式代理结果、TUN 结果。
- `host-after.txt`：status、pf anchors、forwarding 和任何残留进程。

## 中止条件

如果出现以下任一情况，立即停止并收集诊断：

- 主 LAN 上的非测试设备获得了 `192.168.50.x`。
- Mac 上游接口和下游接口是同一个。
- 备用路由器仍在运行 DHCP 或 NAT。
- 上游网络已经使用 `192.168.50.0/24`。
- `stop` 无法卸载 pf anchor 或恢复 IP forwarding。
- 客户端流量只是通过备用路由器自己的 NAT 工作，而不是通过 Mac 网关。
