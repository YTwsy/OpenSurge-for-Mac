# 虚拟 LAN lab

简体中文 | [English](README.md)

这个 lab 会让被测网关继续运行在 macOS 上，并使用两个 Lima Ubuntu VM 作为独立
LAN 客户端。它不会用 Linux 路由器替换 macOS 实现。

```text
Internet
   |
macOS upstream interface
   |
real omg + pf + dnsmasq + mihomo
   |
vmnet host network (192.168.50.0/24, no platform DHCP)
   +-- omg-lab-client-1
   +-- omg-lab-client-2
```

每个客户端都有两个网卡。Lima 内置的用户态网卡继续作为控制和 provisioning
平面使用。第二个网卡是测试数据平面，会向本项目的 dnsmasq 实例申请租约。

## 一次性安装

```sh
make lab-install
```

如需非交互式自动化，也可以安全地拆成两步：

```sh
./tests/lab/install-host-deps.sh --user-only
./tests/lab/install-host-deps.sh --root-only
```

安装器会把固定版本并校验 checksum 的上游 release 下载到 `runtime/tools`，然后：

- 为本项目安装 Lima 2.1.3、dnsmasq 2.93 和 mihomo 1.19.27；
- 校验并把 socket_vmnet 1.2.2 安装到 `/opt/socket_vmnet`；
- 把功能固定的网络 helper 安装到 `/opt/open-mihomo-gateway`；
- 只为 helper 的 `start`、`stop` 和 `status` 命令授予当前用户免密 sudo。

在安装 sudoers 规则前，helper 会被复制到 root 拥有的路径。该规则绝不会执行来自
这个可写仓库的脚本或二进制。运行 `make lab-uninstall-root` 可移除 root 拥有的
helper、socket_vmnet 副本、lab 日志和 sudoers 规则。

可选代理变量可以放在 `runtime/lab/proxy.env`。安装器和 lab 命令会为主机侧操作
加载该文件。默认情况下，Lima VM provisioning 不会接收这些代理变量；只有当代理
端点能从 VM 内访问时，才设置 `OMG_LAB_VM_PROXY=1`。

## 日常流程

```sh
make lab-up
sudo -v
make lab-test
make lab-test-tun
make lab-test-tun-imported-profile
make lab-test-tun-imported-egress
make lab-test-tun-device-policy
make lab-down
```

`lab-up` 会启动没有 DHCP 的 host network 和两个客户端。`lab-test` 会构建当前
网关，用生成的 lab 配置启动它，刷新两个客户端租约，检查路由、DNS、ICMP/NAT、
直连 HTTPS，以及通过 mihomo `mixed-port` 的显式 HTTPS，然后验证清理结果。
artifact 会写入 `artifacts/lab`。managed mihomo DNS 在 TUN 关闭时仍会返回 fake IP，
因此直连 HTTPS 的 NAT 证明会向公共 DNS 取得真实 A 记录，再用 `curl --resolve` 固定
该地址；独立的网关 DNS 断言仍会有意验证 fake-IP 响应。

`lab-test-tun` 是 TUN 透明代理门禁。它会把 lab 配置改写成
`transparent.mode: "tun"`，让 dnsmasq 转发到 mihomo DNS，让客户端不设置显式代理，
并要求无代理 HTTPS 请求出现在 `mihomo.log` 中。

`lab-test-tun-imported-profile` 会使用 imported profile fixture 跑 TUN 门禁。
`lab-test-tun-imported-egress` 会在这条路径上加入本地 HTTP provider 和受控 HTTP
CONNECT proxy，然后通过 `omg policy-select` 把 `TunEgress` 从 `DIRECT` 切到受控
代理。它证明 provider-backed 策略选择会改变透明 TUN 出口路径；它不证明真实订阅
节点或远端出口 IP。

`lab-test-tun-device-policy` 会把两个客户端作为独立识别的 LAN 设备，给它们分配
固定 `.101`/`.102` DHCP 租约，先证明 `dedicated` 设备在全局 `MATCH` 前使用 selector，
再证明 `inherit_global` 设备没有 default selector 且走全局 `MATCH`。脚本制造 desired
drift 后调用真实 `omg reload` 把后者改成独立模式，验证 applied digest 同步、两台设备的
selector 可以互不影响地选择不同出口，再验证设备专属 IP `REJECT`。它是设备身份、
设备路由方式、设备默认出口和设备覆盖的
数据面门禁；还会验证 applied bundle/state digest、两条精确 DHCP identity、编辑 policy
文件后的 desired/applied drift，以及选中 HTTP-only 出口时 UDP/443 必须记录为 `REJECT`
而不能 fall through 到 `DIRECT`。规则、模板和 provider 的编译仍由单元测试覆盖。

请把 `make lab-test` 视为高风险网络改动所需的本地门禁：DHCP/DNS 行为、mihomo
进程或配置生成、pf/NAT 规则、forwarding 和 rollback、网关生命周期清理、lab
拓扑或运行时流量默认值。普通 CI 流程有意停在 `make test`；这个 lab 应运行在开发
Mac、夜间任务或手动控制的 macOS runner 上，并具备同样的 root-owned helper 和隔离
socket_vmnet 网络。

默认 lab 路径会把 `mihomo.redir_port` 和 `pf.redirect_tcp_to` 设为 `0`。当前
Darwin mihomo 构建报告 redir 不受支持，所以透明 TCP 捕获由 TUN 门禁覆盖，而不是
PF TCP 重定向。

网关二进制本身不会获得免密 sudo 规则。运行 `make lab-test` 前请先执行 `sudo -v`，
让测试能使用缓存的 sudo 凭据，同时避免嵌入或扩大 root 权限。

sudo 缓存凭据和终端会话有关，也会过期。如果 agent 或自动化在一个 TTY 里运行
`sudo -v`，却在另一个 TTY 里运行 `make lab-test`，lab 脚本的 `sudo -n` 预检查
仍然可能失败。请在同一个终端会话里、紧挨着 root-required lab 目标之前运行
`sudo -v`。

lab 只应该在 vmnet bridge 上拥有 `192.168.50.1/24`。不要把同一个地址留在其他
接口上。真实设备 smoke 也会在 `en7` 等接口上使用 `192.168.50.1`；运行
`make lab-up` 前请先执行 `make real-device-stop`，或者用
`sudo ifconfig <iface> inet 192.168.50.1 delete` 移除重复地址。重复 LAN IP 会让
macOS 把 DNS 响应路由到错误接口，表现为 TUN lab DNS timeout。

## 命令

```sh
make lab-check    # 显示已安装版本和网络状态
make lab-uninstall-root  # 移除 root-owned lab helper 和 sudoers 规则
make lab-up       # 创建/启动网络和客户端
make lab-status   # 检查主机和客户端状态
make lab-test     # 运行端到端测试并恢复主机
make lab-test-tun # 运行 TUN 透明代理门禁
make lab-test-tun-imported-profile # 使用 imported profile fixture 跑 TUN
make lab-test-tun-imported-egress  # 通过受控代理切换 TUN 出口
make lab-test-tun-device-policy # 验证独立的每设备 TUN 策略
make lab-down     # 停止客户端并移除 host network
make lab-destroy  # 同时删除持久化的 Lima 客户端磁盘
```

可设置 `OMG_LAB_CLIENTS` 改变客户端名称，或设置 `OMG_LAB_TEST_URL` 使用不同的
HTTPS 连通性目标。

## 安全

生成的配置使用 vmnet-backed `bridge` 接口，并且在该接口同时也是默认上游时拒绝
运行。不要把 lab 接口替换成 `en0` 或其他普通 LAN 接口。如果 `192.168.50.1`
配置在非 lab 接口上，`lab-up` 也会拒绝继续。`lab-test` 在断言失败时总会尝试
停止网关并记录诊断信息。
