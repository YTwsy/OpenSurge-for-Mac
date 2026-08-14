---
title: Validation gates
kind: source
status: seed
---

# 验证门槛

`make test` 是快速默认验证门槛。它运行 `go test ./...`，也是当前 CI 级别
检查。

`make lab-test` 是本地 host-network 门槛，服务于高风险网关变更。它在隔离的
socket_vmnet-backed LAN 中，用 Lima 客户端测试真实 macOS gateway，并检查
DHCP、DNS、ICMP/NAT、直连 HTTPS、通过 mihomo `mixed-port` 的显式代理
HTTPS，以及清理行为。

`make lab-test-tun` 是透明代理门槛。它会启用 `transparent.mode: "tun"`，
保持客户端没有显式代理配置，并要求无显式代理的 HTTPS 请求出现在
`mihomo.log` 的透明 TUN 路径中。

`make lab-test-tun-imported-profile` 是 imported profile overlay 的 TUN 门槛。
它使用 `tests/lab/mihomo-profile.imported-tun.yaml`，保持规则为 `MATCH,DIRECT`，
证明 imported profile 可以进入透明 TUN lab 路径。

`make lab-test-tun-imported-egress` 是 imported provider + policy-select 的 TUN
出口切换门槛。它使用本地 HTTP provider 注入 `egress-proxy`，通过
`omg policy-select` 把 `TunEgress` 从 `DIRECT` 切到受控 HTTP CONNECT proxy，并
要求 `mihomo.log` 中的 TUN 目标连接和受控 proxy 日志同时反映切换结果。这个门槛
不证明真实订阅节点、真实远端出口 IP 或 real-device/same-LAN 兼容性。

`make lab-test-tun-local-routing` 是 Mac 本机 Rule/Global/Direct 与下游隔离的
TUN 门槛。它要求本机 TUN source 为 `198.18.0.1`，分别证明本机 Global 使用受控
proxy 时下游仍走 `TunEgress[DIRECT]`，以及本机 Direct 时下游仍可走
`TunEgress[egress-proxy]`。HTTP-only 全局出口必须报告 UDP `reject`，普通
`policies` 不能暴露内部 `open-surge/mac-*` 组。

`make lab-test-tun-device-policy` 是每设备策略的数据面门槛。它让两个 Lima 客户端
分别获得 `.101` 和 `.102` 的 MAC 绑定租约，先对比 `dedicated` selector 与
`inherit_global` 的全局 `MATCH` 路径，并要求跟随设备不存在 default slot；再经真实
reload 将其改成独立模式，要求两台设备的 `device/<id>/default` selector 独立改变
TUN 出口，最后要求设备级 IP `REJECT` 生效。它还要求 applied
snapshot/state digest、一致的 lease identity、desired 文件修改后的 drift，以及 HTTP-only
selector 上 UDP/443 记录为 `REJECT` 而非 fall through 到 `DIRECT`。它覆盖设备身份、
路由模式、设备默认出口和设备覆盖；模板、domain/protocol 组合与 HTTP/MRS rule-provider 的编译由
`make test` 覆盖，不需要为每个操作者规则重复运行 Lab。

IPv6 数据面按拓扑使用 `make lab-test-ipv6-userspace`、
`make lab-test-ipv6-same-wifi` 和 `make lab-test-ipv6-same-lan`。前两条要求两台客户端
通过 dnsmasq RA/SLAAC 获得 `fdfe:dcba:9878::/64` 地址、Medium 优先级默认路由和
RDNSS；第三条要求手工 ULA、Mac link-local 默认网关与 link-local DNS，并证明 dnsmasq 不
发布 RA。三者都要求 TCP、本机受控 UDP request/response 和 QUIC Initial-shaped UDP
carrier 通过 macOS BPF broker、Unix sideband 和 patched Mihomo gVisor 路径按
MAC/InUser 命中各自设备规则。
TCP origin 必须收到 HTTP request，UDP fixture 必须返回固定答案；公网上游不是唯一
捕获证据。stop 必须撤销
自动模式的 default route（或旁路由手工配置）、gateway alias、broker 和 runtime paths；RFC 4862 允许 SLAAC 地址暂时
以 deprecated/等待过期状态保留。QUIC 项只证明 UDP carrier，不宣称完成 HTTP/3
握手。

`make lab-test-ipv6-imported-egress` 是真实订阅与公网 IPv6 的非确定性补充门槛，不能
替代上面的本机受控 fixture。它要求通过 `OMG_LAB_IPV6_REAL_PROFILE` 显式提供 profile，
将其复制为 Lab runtime 下的 mode `0600` 文件，并以 `tun_ipv6: auto` 要求宿主机状态
证明原生公网 IPv6 可用。门槛先用第一台 VM 的 MAC/InUser 域名 `REJECT` 保留身份断言，
再要求它用 `DIRECT` 完成 IPv6-only HTTPS、IPv6 UDP DNS 回包和 QUIC 形态 UDP；HTTPS
回显与 Mac 基线都必须分别属于所选上游接口的 GUA；不同 socket 允许选择不同的 IPv6
隐私地址。随后从 profile 选择不打印名称、且已通过 SOCKS5 UDP ASSOCIATE 公网 IPv6
DNS 回包的实际叶子节点，要求第二台 VM 完成 fake-AAAA HTTPS、公网 IPv6 字面地址
HTTPS、IPv6 UDP DNS 回包和 QUIC 形态 UDP 的 `GLOBAL` 命中。VM 的 ULA 是真实下游
IPv6 包，但不代表运营商 Prefix Delegation/GUA；`DIRECT` 是 Mihomo/gVisor 从 Mac
重新发起连接，不是将 ULA 原样转发到公网。

真实 profile 门槛的 artifact 契约是 fail closed：不得复制订阅、生成的 `mihomo.yaml`、
原始 Mihomo 日志、selector/API 输出或 cache，并要用 profile 中的 server、credential 和
较长节点名 marker 扫描保留文件。正常回滚后删除 runtime secret；若 stop 失败则保留
mode `0600` 的恢复材料并让门槛失败。

## 什么时候必须跑 lab

宣称下列改动具备 runtime 覆盖前，应运行 `make lab-test`：

- DHCP 或 DNS 行为；
- mihomo 进程启动或配置渲染；
- pf/NAT 规则；
- IPv4 forwarding 或 rollback 行为；
- 网关生命周期清理；
- lab 拓扑或测试脚本；
- runtime traffic defaults。

宣称透明代理路径被验证前，应运行 `make lab-test-tun`。

## 运行前置条件

lab 的 root-required 步骤依赖当前终端会话里的 sudo 缓存。`sudo -v` 和
`make lab-test` / `make lab-test-tun` 应在同一个 TTY 里连续运行；如果 agent 在
不同 exec 会话里刷新 sudo，脚本的 `sudo -n` 预检查仍可能失败。

无 controlling tty 的环境（agent exec 会话、CI）可改用 askpass：写一个 700 权限、
向 stdout 输出当前用户密码的 helper 脚本，export `SUDO_ASKPASS` 和
`OMG_LAB_SUDO_ASKPASS` 指向它，再顺序运行 `make lab-up && make lab-test-* &&
make lab-down`。`require_cached_sudo` 会在 `sudo -n true` 失败时内部执行一次
`sudo -A -v`，使认证发生在 lab.sh 同一上下文；`SUDO_ASKPASS` 也让 lab-up/lab-down
里无重定向的 `sudo -n` 自动回退到 helper。helper 含密码，用完立即删除。

虚拟 LAN lab 和真实设备 smoke 默认都使用 `192.168.50.1/24`。运行 lab 前，
这个地址只能存在于 lab 的 vmnet bridge 上。如果 `en7` 等 real-device 下游接口
仍保留 `192.168.50.1`，macOS 可能把 `192.168.50.0/24` 的回程路由选到错误接口，
表现为 `dig @192.168.50.1 example.com A` timeout，而 dnsmasq 日志仍显示收到了
查询。先运行 `make real-device-stop`，或手动删除重复地址。

## 可复用的 Lab 故障边界

- `tests/lab/lima/client.yaml` 的任何变化都会让 Lima 重建客户端。VZ 冷启动可能
  静默约两分钟，再从 vsock SSH 回退到 usernet forwarder 并进入 `READY`。
  在提前终止前检查 `limactl list` 和 `~/.lima/<client>/ha.stderr.log`。
- 每次启动/清理都要把 guest resolver 恢复到 Lima 控制网关，并保证 guest hostname
  可解析。`sudo: unable to resolve host` 或对已停止 `192.168.50.1` 的 DNS 查询
  是 provisioning/control-plane 失败，不是数据面证据。
- `omg0` 必须由 `05-open-mihomo-gateway-lab.network` 在 netplan 生成文件之前单一
  接管。重复默认路由表示存在竞争管理者；IPv6 READY 必须是非 `tentative` / 非
  `dadfailed` 且 `preferred_lft` 为正的 ULA。
- 不可达的 `runtime/lab/proxy.env` 和残缺的专用 Go module cache 都在 patched
  Mihomo 建置前发生。先看 `runtime/lab/logs/mihomo-build.log`；脚本会对 Go mirror
  绕过旧代理，并且只在确认专用 cache 缺文件时清理并重试一次。
- agent 沙箱里的 macOS `sysctl` permission error 是环境权限信号。需要
  host-network 结论时，在已批准的同一 PTY 重跑真实门槛。
- VZ 停止期间的 `use of closed network connection` 只有在没有后续
  `has shut down` / `lab network stopped` 时才算清理失败。
- 只检查当次新生成的 `artifacts/lab/<timestamp>`。每个 IPv6 运行会在开始时
  清除可选 egress fixture 的旧日志，避免用历史命中完成新断言。

## TUN 验收信号

当前 `make lab-test-tun` 的关键信号是：

- 客户端 helper 走 transparent 子命令，而不是显式代理测试；
- 客户端不依赖显式代理配置完成 HTTPS 请求；
- 脚本等待 `mihomo.log` 中出现 `--> <host>:443`；
- 成功时输出类似 `transparent TUN log observed for <host>:443`；
- 测试结束后停止 gateway，并确认 `runtime/lab/state.json` 被移除；
- artifacts 被写入 `artifacts/lab` 以便失败后排查。

`make lab-test-tun-imported-egress` 还应看到：

- `omg providers --format json` 中出现 `tun-egress-provider` 和 `egress-proxy`；
- `TunEgress[DIRECT]` 阶段受控 proxy 没有收到 `CONNECT <host>:443`；
- 执行 `omg policy-select --group TunEgress --policy egress-proxy` 后，`mihomo.log`
  出现 `using TunEgress[egress-proxy]`；
- 受控 proxy 日志出现 `CONNECT <host>:443`。

如果使用历史 lab 结果或人工观察提到 fake-IP DNS 行为，要明确它不是当前脚本
里唯一的直接断言。

## 结论纪律

如果只跑了单元测试，就只说单元测试通过。除非实际运行对应 lab gate，否则不
要暗示已经验证 host-network、root-required 或 transparent-proxy 行为。
