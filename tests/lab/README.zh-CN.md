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
- 默认不安装免密 sudo 规则。

需要无人值守启动/停止隔离网络时，可以明确运行
`./tests/lab/install-host-deps.sh --root-only --with-sudoers`。它只允许当前用户免密执行
root-owned helper 的 `start`、`stop` 和 `status`，绝不会执行来自这个可写仓库的脚本
或二进制；网关二进制本身仍需缓存的 sudo 凭据。运行 `make lab-uninstall-root` 可移除
root-owned helper、socket_vmnet 副本、lab 日志和 sudoers 规则。

可选代理变量可以放在 `runtime/lab/proxy.env`。安装器和 lab 命令会为主机侧操作
加载该文件。默认情况下，Lima VM provisioning 不会接收这些代理变量；只有当代理
端点能从 VM 内访问时，才设置 `OMG_LAB_VM_PROXY=1`。

## 日常流程

```sh
sudo -v && make lab-up
sudo -v && make lab-test
sudo -v && make lab-test-tun
sudo -v && make lab-test-tun-imported-profile
sudo -v && make lab-test-tun-imported-egress
sudo -v && make lab-test-tun-local-routing
sudo -v && make lab-test-tun-device-policy
sudo -v && make lab-down
```

`lab-up` 会启动没有 DHCP 的 host network 和两个客户端。`lab-test` 会构建当前
网关，用生成的 lab 配置启动它，刷新两个客户端租约，检查路由、DNS、ICMP/NAT、
直连 HTTPS，以及通过 mihomo `mixed-port` 的显式 HTTPS，然后验证清理结果。
artifact 会写入 `artifacts/lab`。managed mihomo DNS 在 TUN 关闭时仍会返回 fake IP，
因此直连 HTTPS 的 NAT 证明会向公共 DNS 取得真实 A 记录，再用 `curl --resolve` 固定
该地址；独立的网关 DNS 断言仍会有意验证 fake-IP 响应。

第一次 `lab-up` 需要下载固定 checksum 的 Ubuntu 镜像，并在两个客户端中安装测试
工具，所以会明显较慢。`lab-down` 只停止客户端、保留 Lima 磁盘；日常结束应使用它，
后续 `lab-up` 会复用客户端。只有需要清除损坏状态或有意重建 VM 时才使用
`make lab-destroy`。每次启动时 provisioning 会先把 guest DNS 恢复到 Lima 控制网关，
避免上一次测试遗留的 `192.168.50.1` 在测试网关尚未启动时阻塞启动；依赖已经齐全时
也会跳过 `apt-get update` 和安装。冷重建会串行 provision 两台客户端，避免并发 apt
争抢上游带宽；配置未变化的持久化客户端会并行启动，避免日常 `lab-up` 累加两次 guest
boot 时间。

`lab-test-tun` 是 TUN 透明代理门禁。它会把 lab 配置改写成
`transparent.mode: "tun"`，让 dnsmasq 转发到 mihomo DNS，让客户端不设置显式代理，
并要求无代理 HTTPS 请求出现在 `mihomo.log` 中。

`lab-test-tun-imported-profile` 会使用 imported profile fixture 跑 TUN 门禁。
`lab-test-tun-imported-egress` 会在这条路径上加入本地 HTTP provider 和受控 HTTP
CONNECT proxy，然后通过 `omg policy-select` 把 `TunEgress` 从 `DIRECT` 切到受控
代理。它证明 provider-backed 策略选择会改变透明 TUN 出口路径；它不证明真实订阅
节点或远端出口 IP。受控代理会把上游 DNS 和 TCP 拨号都绑定到物理 upstream
interface，避免代理自身流量重新进入 TUN 或使用 fake IP。

`lab-test-tun-local-routing` 使用同一 imported egress fixture，验证 Mac 本机
规则 / 全局 / 直连和下游客户端彼此隔离：本机 Global 使用受控代理时，下游仍可按
网关规则直连；本机 Direct 绕过代理时，下游仍可按网关规则使用代理。它还验证本机
TUN source、HTTP-only 全局出口的 UDP `REJECT`，以及内部组不会出现在普通
`policies` 输出。

`lab-test-tun-device-policy` 会把两个客户端作为独立识别的 LAN 设备，给它们分配
固定 `.101`/`.102` DHCP 租约，先证明 `dedicated` 设备在全局 `MATCH` 前使用 selector，
再证明 `inherit_global` 设备没有 default selector 且走全局 `MATCH`。脚本制造 desired
drift 后调用真实 `omg reload` 把后者改成独立模式，验证 applied digest 同步、两台设备的
selector 可以互不影响地选择不同出口，再验证设备专属 IP `REJECT`。它是设备身份、
设备路由方式、设备默认出口和设备覆盖的
数据面门禁；还会验证 applied bundle/state digest、两条精确 DHCP identity、编辑 policy
文件后的 desired/applied drift，以及选中 HTTP-only 出口时 UDP/443 必须记录为 `REJECT`
而不能 fall through 到 `DIRECT`。门禁还保留一条没有 MAC 的原始设备记录，验证 DHCP
模式会把它保存在 applied snapshot 中供以后补充身份，但不会生成租约、mihomo 规则或
活动 selector。通过时的 applied snapshot、runtime state、dnsmasq/mihomo 生成配置和
初始/重载后设备视图会一起写入 artifact，便于复核这条边界。规则、模板和 provider 的
编译仍由单元测试覆盖。

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
`sudo -v`；最稳妥的形式是 `sudo -v && make <lab-target>`，长时间运行多个门禁时每个
目标都重新验证一次，不要把一次缓存视为整个 Lab 会话永久有效。冷启动的 `lab-up`
本身可能超过 sudo ticket 的有效期，因此它完成后必须再次执行
`sudo -v && make lab-test...`；清理前如果已经过了较长时间，也用
`sudo -v && make lab-down`，否则 VM 可能已停止但 root-owned helper 的状态文件仍残留。

固定大小的 Lima/mihomo 下载采用分段缓存并在合并后校验 checksum。下载因 TLS 或网络
波动失败时，直接重跑同一条安装命令；安装器会复用完整分段或续传未完成分段，不要手工
把未校验的文件复制进 `runtime/tools/cache`。

默认客户端规格是 `1 CPU / 512 MiB`。仅凭 `lab-up` 很慢，不要先上调资源；先用
`limactl shell <client> -- free -m`、`vmstat` 和 guest 的 OOM 日志区分 CPU/内存压力与
DNS、镜像下载、apt provisioning 等等待。CPU 大部分时间 idle、仍有 available memory
且没有 OOM 时，提高 CPU/内存不会解决启动慢；只有出现持续高 load、明显内存回收或 OOM
证据时才调整默认规格。

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
make lab-test-tun-local-routing # 验证 Mac 本机模式与下游隔离
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
