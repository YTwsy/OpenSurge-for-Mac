# Agent 指南

OpenSurge for Mac 是一个开源的 Surge for Mac 风格 macOS 网关与控制面。
v0.3 面向用户的入口是 Wails + 系统 WebView 桌面 App，复用 React 主界面与
菜单栏面板；浏览器 Web GUI 继续受支持。`omg`
CLI 保留为运维、诊断、自动化和恢复接口。核心能力是全屋代理网关：Mac 为
下游设备承担网关职责，并按拓扑提供 DHCP/DNS；mihomo 作为当前代理引擎，
macOS 网络能力负责 NAT、转发与透明路由。

这个文件是 coding agent 进入本仓库时的第一站。凡是改动网关行为、网络
验证、配置语义或项目定位，都应先读这里。

## 按问题定位

- 了解公开范围和用户操作时读 `README.md` 或 `docs/user/README.md`。
- 从 `docs/agent-wiki/wiki/index.md` 定位来源或代码；已知入口可直接进入，需要理解跨主题关系才读 Concept。
- 网关启停、回滚与资源所有权见 `docs/agent-wiki/sources/decisions/gateway-lifecycle.md`；透明接入与下游 IPv6 见同目录的 `tun-mainline.md` 和 `downstream-ipv6-takeover.md`。
- 选择检查见 `docs/agent-wiki/sources/validation/test-gates.md`。回答支持或验收问题时，从同目录 `evidence-map.md` 定向读取对应记录并核对版本、环境与路径。

## 产品方向

- 产品身份是 `OpenSurge for Mac`。
- 当前代理引擎是 `mihomo`，它不是产品名。
- 核心网关模型是：dnsmasq 提供 DHCP/DNS，mihomo 提供代理能力，pf
  提供 NAT，sysctl 管理 macOS IPv4 forwarding 状态。
- 实验性的下游 IPv6 使用 dnsmasq RA/SLAAC/RDNSS 或手工 ULA 接入，并通过
  macOS BPF broker 与本项目补丁构建的 mihomo `opensurge-packet`/gVisor 数据面处理。
- React 主窗口 / Web GUI 是主要操作控制面；菜单栏面板负责状态、恢复提醒和入口，CLI 负责
  运维、诊断、自动化与恢复。三者应复用现有 Go 业务规则，不建立平行业务实现。
- 工程方向是：Mac-native、可审计、带透明路由，并以可复现实验室验证约束
  高风险网络能力。

不要把产品重新命名为 mihomo。`omg` 与 `open-mihomo-gateway` 是当前实现期
遗留的技术命名，除非任务明确要求迁移，否则不要在品牌层面扩大它们。

## 网络规则

- TUN 是 macOS 上受支持的透明代理路径。
- 除非项目明确重新打开该决策，否则 `mihomo.redir_port` 与
  `pf.redirect_tcp_to` 必须保持 inactive。
- 下游 IPv6 ingress 不进入 macOS 系统 TUN，但仍要求整体
  `transparent.mode: "tun"`；共享 L2 必须消除竞争 IPv6 RA/默认路由。
- 高风险网络改动需要实验室验证，不能只依赖单元测试。
- 结论必须精确说明实际运行了哪些对应门槛。

## 验证

`make test` 是快速默认门槛，当前等价于 `go test ./...`，也是 CI 级别门槛。

涉及 DHCP、DNS、mihomo 进程或配置生成、pf/NAT、IPv4 forwarding、rollback、
网关生命周期清理、lab 拓扑或 runtime traffic defaults 的改动，
需要用 `make lab-test` 才能宣称真实 host-network 路径被验证。

涉及透明代理的改动，需要用 `make lab-test-tun` 才能宣称 TUN 透明代理路径被
验证。这个门槛会保持客户端无显式代理配置，并要求 HTTPS 流量出现在 mihomo
TUN 路径的日志中。

涉及下游 IPv6 RA/SLAAC/RDNSS、BPF broker、patched mihomo packet listener、设备身份
或停止撤销的改动，按拓扑运行 `make lab-test-ipv6-userspace`、
`make lab-test-ipv6-same-wifi` 或 `make lab-test-ipv6-same-lan`，才能宣称对应
host-network IPv6 路径已验证。

如果沙箱阻止 Go cache 写入，把 `GOCACHE` 指向 `/private/tmp` 下的路径。

## Agent Wiki 维护规则

`docs/agent-wiki/sources/` 维护项目规范、领域契约、验证方法与有范围的证据；
`wiki/concepts/` 维护理解系统所需的概念、关系、职责边界和来源入口，必要时使用简单结构图。
具体字段、行为规则、实现步骤和验证要求归入对应 source；精简前先迁移独有的有效规则并核对差异。

长期变化随代码更新主要来源；只有概念关系或入口变化时才同步 Concept、索引与本文件。
面向用户的操作说明放在 `docs/user/`。历史背景按需追溯，不作为当前行为或当前验收的依据。
临时想法、普通 TODO 和完整一次性日志保留在当前上下文或现有工作记录中。

`sources/validation/` 只维护可复用方法、通过标准与已知证据范围。单次执行的版本、环境、
实际检查、结果和未覆盖项写入提交/PR 或已有任务记录，不为每次检查新增 validation 页面。
影响能力判断的新结果、阻塞或复验要求同步到 `evidence-map.md` 与对应领域限制；
未收录不等于不支持，最近已知通过不等于当前版本已复验。

默认不建任务文件。需要跨会话接续时使用 `docs/agent-wiki/tasks/<id>.md`，复杂调查才拆目录。
已有任务与单次报告结束后移入 `docs/agent-wiki/tasks/finished_archived/`；先提炼有效规则，
并保留当前证据入口到重要结果和未覆盖项的直接链接。没有记录不为归档补建任务台账。

普通递归检索遵守 `.ignore`。验收判断、回归或版本追溯时沿证据链接定向读取归档，
必要时使用 `rg --no-ignore`；不默认展开历史。归档由 Git 保留，不加入 `.gitignore`。
