# Virtual Lab 运行前提与故障边界

本页维护 Lab 环境和可复用排障方法；具体功能命令与通过标准见 [验证门槛](test-gates.md)，历史结果见 [证据入口](evidence-map.md)。

## 权限与启动

root-required Lab 目标应在同一个 TTY 里用 `sudo -v && make <lab-target>` 启动。
macOS sudo 缓存既会过期，也可能因 TTY/执行上下文不同而无法被脚本中的 `sudo -n`
复用；长时间连续跑多个门禁时，每个目标前都重新验证。除非运行环境明确需要无人值守，
不要把临时凭据问题扩大成宽泛的免密 sudo；仓库提供的可选规则也只限 root-owned
network helper 的三个固定子命令，不能代替网关测试所需的 sudo 缓存。
冷 `lab-up` 可能比 sudo ticket 活得更久，因此它完成后必须再次验证再启动测试；长门禁
结束后的 `lab-down` 同理。VM 停止但 helper stop 报 `sudo: a password is required` 时，
应把它视为不完整清理并重新执行带 `sudo -v` 的 `lab-down`。

无 controlling tty 的环境（agent exec 会话、CI）可改用 askpass：写一个 700 权限、
向 stdout 输出当前用户密码的 helper 脚本，export `SUDO_ASKPASS` 和
`OMG_LAB_SUDO_ASKPASS` 指向它，再顺序运行 `make lab-up && make lab-test-* &&
make lab-down`。`require_cached_sudo` 会在 `sudo -n true` 失败时内部执行一次
`sudo -A -v`，使认证发生在 lab.sh 同一上下文；`SUDO_ASKPASS` 也让 lab-up/lab-down
里无重定向的 `sudo -n` 自动回退到 helper。helper 含密码，用完立即删除。

## VM、依赖与恢复

Apple Silicon 上的 Codex/agent 终端可能经 Rosetta 运行，使 `uname -m` 显示
`x86_64` 并触发安装器架构拒绝。确认 `sysctl.proc_translated=1` 和
`hw.optional.arm64=1` 后，用
`/usr/bin/arch -arm64 /bin/bash ./tests/lab/install-host-deps.sh` 运行安装器；Intel Mac
不能使用这条绕行命令。

第一次 `lab-up` 包含固定镜像下载和 guest 依赖安装，不能和持久化 VM 的后续启动耗时
直接比较。正常清理使用 `lab-down` 保留磁盘，只有损坏或有意重建时使用 `lab-destroy`。
guest 的数据面 DNS 在一次测试后会指向 `192.168.50.1`；而下一次 `lab-up` 时被测网关
尚未运行，所以 provisioning 必须先恢复 Lima 控制面 DNS，并在依赖已齐全时跳过 apt，
否则会表现为 UDP/53 connection refused 与很慢的 boot scripts。
冷重建保持串行 provisioning，稳定复用的 VM 则并行启动；这样既不让两个 apt 任务争抢
上游带宽，又避免日常启动累加两次独立 guest boot 时间。
修改 `tests/lab/lima/client.yaml` 会使 Lima 按精确配置比较删除并重建对应
VM，这是有意的冷启动。VZ 冷启动可能在约两分钟内没有新输出，然后从
vsock SSH 回退到 usernet forwarder 并进入 `READY`；不要只因为这段静默就杀掉进程。
先看 `runtime/tools/lima/bin/limactl list` 和 `~/.lima/<client>/ha.stderr.log`。

Lab 环境问题必须与数据面失败分开记录：

- 公网 HTTPS 应先选择当前网络能无代理直连的 `OMG_LAB_TEST_URL`，再在门槛中
  保持该目标一致。受控 CONNECT 夹具可用 `OMG_LAB_EGRESS_HTTP_PROXY=host:port`
  指定已预检可达的 LAN 代理上游；它不改变客户端、系统 DNS 或 DIRECT 分支，
  也不能作为受控代理原生直连出口的证据。公网站点和外部 DNS 的可达性不能替代
  或否定已观测到的本地路由、身份和受控 HTTP/3 证据。

- 启动和清理会把 guest `/etc/resolv.conf` 恢复到 Lima 控制网关，并保证本机
  hostname 可解析。如果 provisioning 报 `sudo: unable to resolve host` 或仍向已停止的
  `192.168.50.1` 查询，先运行 guest helper 的 `restore-control`，不要把它算作
  IPv6 数据面结果。
- `omg0` 由 `/etc/systemd/network/05-open-mihomo-gateway-lab.network` 单一接管。
  `networkctl status omg0` 应显示该文件；重复 IPv4/IPv6 默认路由通常意味着
  netplan 和手工 DHCP/RA 同时在管理接口。IPv6 READY 信号还必须排除
  `tentative` / `dadfailed` 地址，并要求正的 `preferred_lft`。
- 自动 RA 模式不保证 `/etc/resolv.conf` 直接出现 IPv6 nameserver。IPv6 client
  probe 应在没有显式 IPv6 nameserver 时使用 `omg0` IPv6 默认路由的 link-local
  next hop，并附加接口 scope；空的 HTTP/3 client evidence 通常先检查这一点。
- quic-go 在最小 guest 中可能报告无法把 UDP receive buffer 增大到建议值。若随后有
  `CLIENT_IPV6_HTTP3_OK`，这是吞吐告警而不是握手失败；这个功能门槛不证明 QUIC 性能。
- `runtime/lab/proxy.env` 中不可达的旧代理和专用 `/private/tmp` Go module
  cache 残缺都是 patched Mihomo 的构建前故障。脚本会对 Go mirror 绕过旧代理，
  并在日志确认是该专用 cache 的缺文件后清理并重试一次。先查
  `runtime/lab/logs/mihomo-build.log`，不要进入数据面调试。
- agent 沙箱中的 `sysctl kern.bootsessionuuid` / `kern.boottime` 或
  `sysctl.proc_translated: operation not permitted` 是执行环境权限信号。需要
  host-network 结论时，在已批准的同一 PTY 重跑对应门槛。
- Lima 停止 VZ 时可能在红色日志中打印 `use of closed network connection`。
  如果后续同时出现 `has shut down` 和 `lab network stopped`，这是 hostagent 关闭
  listener 后的收尾噪声，不是清理失败。

不要仅凭启动耗时把默认 `1 CPU / 512 MiB` 判定为不足。先采集 guest 的 available
memory、load、CPU idle/iowait 和 OOM 记录；如果 CPU 主要 idle、内存仍可用且没有 OOM，
应优先排查 DNS、下载和重复 provisioning，而不是增加 VM 常驻资源。

## 地址与证据隔离

- `192.168.50.1` 只配置在当前 lab bridge 上；virtual LAN 使用 `/22`，真实设备
  smoke 默认仍使用 `/24`。如果 `en7` 等接口残留 `192.168.50.1/24`，macOS 可能把
  重叠范围内的 lab client 回程路由到
  错误接口，表现为 TUN DNS timeout。先运行 `make real-device-stop` 或删除重复
  地址。

只检查当次新生成的 `artifacts/lab/<timestamp>`。每个 IPv6 运行开始时清除可选 egress fixture 旧日志，避免历史命中参与新断言。正常清理后运行 `make lab-status` 核对状态；清理失败保留恢复材料并报告。

历史人工观察中的 fake-IP DNS 不能代替当前脚本的直接断言。所有通过声明仍需写出本次实际运行的 gate 和被测版本。
