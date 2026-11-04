# Mihomo 1.19.33 网络回归

状态：完成。10 项网络门槛、正式 DNS 引用矩阵及 5 项 Lab 隔离夹具通过；
Lab 已清理，原安装版网关已恢复。此次没有修改产品运行代码或执行 PKG 安装。

## 范围与版本

- 日期：2026-11-04（Asia/Shanghai）；macOS 14.8.5，Apple Silicon，Go 1.25.3。
- 目标制品：OpenSurge 0.3.3 build 48，源码 `f012f64911c82a9936d416ef3ee898d79fa471f5`。
  [本地 PKG](../../../../artifacts/gui-installer/OpenSurge-for-Mac-0.3.3-arm64-build48-f012f64-unsigned.pkg)
  保持原样，SHA-256 为 `c4c3d51d98da888946ea36304ffd8dd4c5e3b607c066726174172923d4d519d1`。
- 核心：`1.19.33-opensurge.1`，上游 `3f27ef39f8c96bbfbc8b0b9bb10f5c9ef60149d8`；
  arm64 二进制 SHA-256 为 `9f20c6ff336fd1786cb1bad1b4a80229800ea7510bd90ca1c2bae665998af218`。
- 下表全部门槛显式使用 PKG 解出的同一 arm64 核心。运行基线
  `f9d07aff599e926cf53bec4ef7383f8027135fbc` 相对制品提交只有文档变化；
  本轮工作区差异限于 Lab 脚本、夹具、CI 与文档。产品源码与制品提交一致。
- Lima 2.1.3、socket_vmnet 1.2.2、dnsmasq 2.93，两台 Ubuntu arm64 虚拟客户端。
  公网 HTTPS 使用通过原生直连预检的 `https://www.apple.com/`，未设置受控出口的 LAN 代理。

用户授权暂停安装版网关并在测试后恢复。各轮在同一 sudo TTY 中保活并执行，
退出处理负责 Lab stop/down/status 和安装版恢复；VM 准备期间仍保持原网关运行。
原安装版为 build 46 / `1.19.30-opensurge.1`，最终恢复的也是这一版本。

## 检查结果与证据

| 检查 | 结果 | 证据目录（仓库 `artifacts/lab/` 下） |
| --- | --- | --- |
| `lab-test` | 通过 DHCP/DNS、ICMP/NAT、直连与 mixed-port HTTPS | `20261104-211952` |
| `lab-test-tun` | 通过无显式代理 HTTPS 和真实 TUN 日志 | `20261104-211958` |
| `lab-test-tun-imported-profile` | 通过导入 profile 与透明流量 | `20261104-212005` |
| `lab-test-tun-imported-egress` | 通过 DIRECT/受控 CONNECT 出口切换 | `20261104-212015` |
| `lab-test-policy-workspace` | 通过准备态交接、选择保留和真实 TUN 流量 | `20261104-212024` |
| `lab-test-tun-device-policy` | 通过双设备、跨第三段 /22、reload、连接刷新及 UDP fail closed | `20261104-212922` |
| `lab-test-tun-local-routing` | 通过系统 DNS、核心重启、Rule/Global/Direct、本机 IPv4/IPv6 TCP 与 HTTP/3、下游隔离 | `20261104-215603` |
| `lab-test-ipv6-userspace` | 通过隔离 LAN IPv6 拓扑 | `20261104-214748` |
| `lab-test-ipv6-same-wifi` | 通过同 Wi-Fi DHCP IPv6 拓扑 | `20261104-214821` |
| `lab-test-ipv6-same-lan` | 通过同网段手工 IPv6 拓扑 | `20261104-214845` |

三个 IPv6 门槛分别检查 RA/SLAAC/RDNSS 或手工 ULA/link-local 接入、真实双向 BPF、
TCP/UDP、QUIC carrier、HTTP/3-only DIRECT 与 SOCKS5 UDP，以及 HTTP-only 出口拒绝。
同时核对双栈设备身份、连接归属、绕行设备的 IPv6 阻断和对应拓扑的停止清理。

正式 DNS 1053 引用矩阵使用同一核心执行：

```sh
OMG_PREPARED_MIHOMO_BINARY="$PKG_CORE" OMG_DNS_REFERENCE_GATEWAY_CORE=1 \
  go test ./internal/mihomo -run '^TestDNSReferencesRealCore$' -count=1 -v
```

结果为 21 个 test/subtest 通过事件，无跳过。`make lab-unit-test` 的 5 项隔离夹具通过；
shell/provisioning 语法与 `git diff --check` 通过。单元夹具不接触宿主网络，已接入 CI。

制品旁的 [network-regression.json](../../../../artifacts/gui-installer/OpenSurge-for-Mac-0.3.3-arm64-build48-f012f64-unsigned.network-regression.json)
记录每个门槛的目录、核心身份、清理及恢复结果；
[build-info.json](../../../../artifacts/gui-installer/OpenSurge-for-Mac-0.3.3-arm64-build48-f012f64-unsigned.build-info.json)
保留原构建时证据，并单独关联构建后的网络验证。
原始日志、配置快照与失败现场仅保留在本机私有目录及未跟踪的 Lab artifacts 中，未纳入 Git。

## 测试方式修正与失败归因

1. **失效 bridge 被误判为可复用。** 已有 socket_vmnet 进程与 socket，但 bridge 缺失时，
   helper 曾提前返回成功。改为重建其拥有的 Lab 网络；健康 bridge 保持复用并补回
   interface 记录。两项无 root 夹具覆盖这两个分支，旧 helper 可复现失败，修复后通过。
2. **guest 隐式继承旧代理，依赖检查不准确。** Lima 默认传播宿主 proxy 环境，绕过了
   Lab 显式开关；首次 provisioning 还尝试安装不需要的 isc-dhcp-client。
   模板关闭隐式传播，按实际可执行命令检查依赖，继续使用 systemd-networkd。
   `lab-up` 在 SSH 就绪后检查工具和 helper/networkd 文件；冷重建后验证无继承代理、
   依赖齐全、provisioning 正常退出，并完成真实门槛。
3. **macOS 默认解析器会过滤 AAAA。** 新旧核心均因默认 `getaddrinfo` 只返回 fake A
   而触发旧断言（`20261104-212038`、`20261104-212840`、`20261104-212846`）。
   Darwin SDK `netdb.h` 说明零 flags 隐含 `AI_DEFAULT` / `AI_ADDRCONFIG`；当前无原生
   IPv6 环境会筛选地址族。诊断确认显式 `AI_CANONNAME` 或 `AI_UNUSABLE` 经同一系统
   解析器可返回 fake A/AAAA。门槛保留默认 fake A 断言，以 `AI_CANONNAME` 校验双栈，
   并继续要求 UDP/TCP DNS 捕获和真实 TUN IPv6 流量。修正后的 DNS 断言在新旧核心
   重启前后均通过；完整本机路由最终在候选核心通过。
4. **核心重启后的私有日志无法由 runner 读取。** `umask 077` 下，root 重启核心创建
   0600 日志（`20261104-213451`）。门槛仅把这个临时 Lab 日志的所有者交回 runner，
   保留私有权限，产品日志行为不变。
5. **IPv6 runner 忽略显式核心路径。** 原代码无条件调用构建器，Rosetta 终端的
   `uname -m` 使其选择 amd64。最初三个 IPv6 通过记录为 `20261104-213002`、
   `20261104-213035`、`20261104-213102`，属于 amd64/Rosetta 补充证据，不能用于
   验收 arm64 制品。现在 IPv6 与其他门槛一致尊重 `OMG_LAB_MIHOMO_BINARY`，
   无效路径直接失败；新增三项夹具覆盖显式路径、拒绝回退和默认构建。
   artifacts 新增版本、架构和摘要。上表三个 IPv6 门槛已使用确切 PKG arm64 核心复跑，
   三份身份记录均匹配制品摘要。
6. **接管前缓存的真实 A 干扰域名规则断言。** 候选核心已成功下载 HTTPS 内容，但
   无嗅探时真实 IP 不携带预期域名身份，旧本机探针因此漏掉预期日志（`20261104-214917`）。
   IPv4 packet probe 改为向当前核心请求 fake A，再经 `curl --resolve` 固定该映射；
   仍走真实 TUN，无显式代理，规则/出口/下游隔离断言保留。系统解析器由独立随机域名
   检查覆盖，IPv6 probe 也继续使用当前 fake AAAA。修正后完整本机门槛通过。

另外，一次原生公网 HTTPS 预检超时发生在门槛前；没有把它计为核心或数据面失败。
最终各门槛使用预检可达的同一目标。以上问题均定位到环境或测试方式，本轮未发现
需要改动产品运行代码或 OpenSurge 核心补丁的失败。可复用方法已归入
[Lab 运行前提与故障边界](../../sources/validation/lab-gates.md)。

## 清理与恢复

最终门槛于 21:56:04 通过，21:56:08 完成 Lab 清理，21:56:12 完成原网关恢复。
退出处理在之前各轮失败后也完成了恢复；没有留下失败测试占用网关的状态。

- `lab-down` 成功，`lab-status` 显示网关、DHCP、核心、TUN 与两台 VM 均停止，PF 已卸载。
- Lab state、packet socket/broker socket、vmnet socket/interface 记录均不存在；
  原网关恢复前 `utun123` 不存在，IPv4 forwarding 为 0。
- 原安装版配置文件 SHA-256 前后一致。恢复前后比较 gateway、runtime、DHCP、核心版本、
  TUN、TUN interface、PF、forwarding、IPv4/IPv6 接管和系统 DNS 共 11 个字段，无差异。
- 原网关恢复为 active/running，DHCP 和 `1.19.30-opensurge.1` 运行，TUN ready、PF loaded、
  forwarding enabled、IPv4 takeover ready；原本关闭的下游 IPv6 仍关闭。
  随后的独立只读检查再次确认状态，并通过 DNS 1053 fake A 与无显式代理 HTTPS 200。
- 两台 Lab VM 的磁盘保留供复用；运行时网络资源已清理。PKG 摘要复核仍一致，未安装新包。

## 证明边界

结果限定于 Apple Silicon macOS 主机和两台受控虚拟客户端。没有验证物理客户端、
实际 Wi-Fi 竞争 RA、路由器断链/睡眠/重启、真实订阅节点或 Tailnet/Exit Node，
亦没有原生 Intel 硬件验收。受控 HTTP/3 不证明所有 QUIC 版本、迁移、0-RTT、PMTUD
或公网代理组合。本机 IPv6 开关关闭的配置已有隔离核心检查，本轮网络门槛使用开启状态。
Lab 使用与制品一致的产品源码构建运行工具；本轮没有验证安装器执行或桌面 UI 安装流程。
