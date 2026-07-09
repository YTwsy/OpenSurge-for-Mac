# OpenSurge for Mac

简体中文 | [English](README.md)

OpenSurge for Mac 是一个 macOS 网关项目。它的目标是把一台 Mac 变成可控的
IPv4 LAN 网关，为手机、平板、虚拟机、测试设备和其他下游客户端共享带代理能力
的网络连接。

项目目标更大：做一个 Mac 原生、可审计的网关，支持透明路由、可复现的 lab 验证，并逐步演进出更友好的
控制界面。

## 当前范围

当前实现是一个 CLI 驱动的 MVP：

1. CLI、配置、运行时状态，以及文本/JSON 形式的 status/doctor/logs/snapshot
   命令。
2. dnsmasq 配置、进程管理和租约解析。
3. mihomo 配置、进程管理、版本 API 检查，以及通过 mihomo
   external-controller API 进行策略组选择。
4. pf anchor 管理和 IPv4 forwarding 恢复。

今天的 OpenSurge for Mac 可以：

- 通过 CLI 准备和检查网关配置；
- 启停 DHCP/DNS、mihomo、pf NAT 和 IPv4 forwarding，并带有 rollback；
- 支持通过 mihomo `mixed-port` 进行显式代理；
- 支持 macOS 上基于 mihomo TUN 的透明代理；
- 在 mihomo 运行时，通过 CLI 查看和切换 mihomo 策略组；
- 通过 CLI 查看当前 mihomo 连接状态；
- 通过 CLI 查看运行时日志路径，以及最近的 dnsmasq/mihomo 日志行；
- 为 UI 和诊断控制面收集允许局部失败的 JSON snapshot；
- 在接触普通 LAN 前，先用隔离的虚拟 LAN 验证高风险网络行为。

## 透明代理

macOS 上支持的透明代理路径是 TUN。mihomo `redir-port` 和 PF TCP 重定向被
有意禁用，因为当前 Darwin 构建在运行时报告 redir 不受支持。请保持
`mihomo.redir_port` 和 `pf.redirect_tcp_to` 为 `0`，并通过
`transparent.mode: "tun"` 启用透明代理。

## mihomo profile

OpenSurge for Mac 可以渲染托管的 mihomo 配置，也可以从已有 mihomo profile
导入代理和规则 section。在 imported 模式下，OpenSurge 仍然接管网关关键字段：
LAN 绑定、`allow-lan`、DNS、TUN、`external-controller` 和 runtime 路径。
导入的 profile 只贡献 `proxies`、`proxy-providers`、`proxy-groups`、
`rule-providers` 和 `rules` 这些引擎层 section。

```yaml
mihomo:
  profile_mode: "imported"
  profile: "./profiles/home.yaml"
```

相对形式的 `mihomo.profile` 会基于 OpenSurge 配置文件所在目录解析。导入的
`proxy-providers` 和 `rule-providers` 内部如果有相对 `path:`，会基于被导入的
mihomo profile 所在目录解析。OpenSurge 会渲染 `profile.store-selected: true`，
让 mihomo 可以跨重启保存策略组选择。

启动网关服务前，可以先预览最终生成的 mihomo 配置：

```sh
go run ./cmd/omg doctor --config examples/config.imported-profile.example.yaml
go run ./cmd/omg render-mihomo --config examples/config.example.yaml
go run ./cmd/omg render-mihomo --config examples/config.imported-profile.example.yaml
```

当 `mihomo.binary` 指向已安装的 mihomo 二进制时，可以使用
`validate-mihomo`。它会渲染最终配置，并运行 mihomo 自己的 `-t` 校验，但不会
启动网关服务。

```sh
go run ./cmd/omg validate-mihomo --config examples/config.imported-profile.example.yaml
```

## 使用

```sh
go run ./cmd/omg doctor --config examples/config.example.yaml
go run ./cmd/omg status --config examples/config.example.yaml
go run ./cmd/omg status --config examples/config.example.yaml --format json
go run ./cmd/omg logs --config examples/config.example.yaml --tail 50 --format json
go run ./cmd/omg snapshot --config examples/config.example.yaml --tail 50 --format json
go run ./cmd/omg policies --config examples/config.imported-profile.example.yaml
go run ./cmd/omg policy-select --config examples/config.imported-profile.example.yaml --group Proxy --policy DIRECT
go run ./cmd/omg connections --config examples/config.imported-profile.example.yaml --format json
go run ./cmd/omg render-mihomo --config examples/config.example.yaml
sudo go run ./cmd/omg start --config examples/config.example.yaml --format json
sudo go run ./cmd/omg stop --config examples/config.example.yaml --format json
```

`policy-select` 会先读取 live mihomo 策略组，并在发送切换请求前拒绝未知 group
或 policy。`logs --tail N --format json` 会包含最近的 `dnsmasq` 和 `mihomo`
日志行，并为每个日志文件返回存在状态和读取错误字段，方便控制面消费。
`snapshot --format json` 会聚合 status、doctor、leases、日志尾部、策略组和
连接；mihomo API 失败会留在 `mihomo` 字段内部，不影响其余 snapshot 可用。
`start --format json` 和 `stop --format json` 会在网关动作成功后返回包含
`command`、`ok` 和 `config_path` 的成功 payload。

## 安全

`start` 和 `stop` 需要用 `sudo` 运行，因为它们会管理 DHCP、pf 和 IPv4
forwarding。运行时文件会写入配置文件中的 `runtime.dir`。

## 开发流程

把 `make test` 作为快速默认门禁。CI 当前只运行这个单元测试门禁，所以普通
push 和 pull request 不需要主机网络、免密 sudo、Lima 或 socket_vmnet。

在提交或评审高风险网络改动前，请本地运行 `make lab-test`。这包括 DHCP、
DNS、mihomo 启动/配置渲染、pf 规则、forwarding/rollback 行为、网关生命周期、
lab 脚本，以及会影响运行时流量的示例配置。除非有专用 macOS runner 能提供同样
受控的主机权限和网络隔离，否则虚拟 LAN lab 应保持为本地、夜间或手动门禁。

使用 `make lab-test-tun` 验证支持的透明代理路径。该测试会让客户端不配置代理，
并要求 mihomo 日志中出现通过 TUN inbound 观察到的直连 HTTPS 请求。修改
mihomo profile 导入或 overlay 行为时，使用
`make lab-test-tun-imported-profile`；它会用 imported profile fixture 跑同一个
TUN 门禁。

策略组控制面和机器可读 CLI 改动优先使用 `make policy-control-test`。它会启动真实
mihomo 二进制，但不使用 sudo、dnsmasq、pf 或 TUN，并通过 live external-controller
API 检查 `policies`、`policy-select`、mihomo 重启后的策略选择恢复，以及
`connections` 和 `snapshot`；其中也会验证未知 policy 会被 `policy-select`
拒绝。

使用 `make same-lan-start-tun` 和 `make same-lan-adb-check` 验证窄范围的同
LAN 默认网关 smoke。这个 gate 会保持 DHCP disabled，要求 TUN，并通过 ADB 检查
一台默认网关和 DNS 指向 Mac LAN IP 的 Android 测试设备。需要先验证单个域名的
真实代理出口时，可以配合 `OMG_SAME_LAN_*` 上游代理环境变量使用
`make same-lan-start-tun-proxy`，例如先测 `api.ipify.org`，再讨论完整订阅导入。
它不宣称已经具备全 LAN 上线能力或策略组切换能力。

## 虚拟 LAN lab

集成 lab 会用两个轻量 Linux 客户端测试真实的 macOS 网关。Lima 提供客户端，
socket_vmnet 创建一个没有竞争 DHCP 服务器的隔离二层主机网络。测试覆盖 DHCP、
DNS、ICMP/NAT、直连 HTTPS，以及通过 mihomo `mixed-port` 的显式 HTTPS。

```sh
make lab-install
make lab-up
sudo -v
make lab-test
make lab-test-tun
make lab-test-tun-imported-profile
make lab-down
```

一次性安装器会添加一个 root 拥有、功能固定的网络 helper，并添加一个很窄的
sudoers 规则，只允许启动、停止和查看 lab 网络状态。网关二进制本身不会获得免密
root 权限；端到端测试前请用 `sudo -v` 刷新 sudo ticket。拓扑、安全检查和排障
步骤见 `tests/lab/README.zh-CN.md`。
