# Agent 指南

OpenSurge for Mac 的最终目标，是成为一个开源的 Surge for Mac 风格
macOS 网关与控制面。当前 CLI 只是 MVP 形态；真正的核心能力是全屋代理
网关：Mac 为下游局域网提供 DHCP/DNS，mihomo 作为当前代理引擎，macOS
网络能力负责 NAT、转发与透明路由。

这个文件是 coding agent 进入本仓库时的第一站。凡是改动网关行为、网络
验证、配置语义或项目定位，都应先读这里。

## 先读这些

1. 读 `README.md`，了解面向用户的范围和当前 CLI 工作流。
2. 读 `docs/agent-wiki/wiki/index.md`，获取 agent 专用项目上下文。
3. 如果改网关行为，读
   `docs/agent-wiki/wiki/concepts/gateway-lifecycle.md`。
4. 如果改透明代理，读
   `docs/agent-wiki/wiki/concepts/macos-tun-transparent-proxy.md`。
5. 如果判断测试或验收门槛，读
   `docs/agent-wiki/wiki/concepts/validation-gates.md`。

## 产品方向

- 产品身份是 `OpenSurge for Mac`。
- 当前代理引擎是 `mihomo`，它不是产品名。
- 核心网关模型是：dnsmasq 提供 DHCP/DNS，mihomo 提供代理能力，pf
  提供 NAT，sysctl 管理 macOS IPv4 forwarding 状态。
- 长期方向是：Mac-native、可审计、带透明路由、可复现实验室验证，并逐步
  具备更友好的控制面。

不要把产品重新命名为 mihomo。`omg` 与 `open-mihomo-gateway` 是当前实现期
遗留的技术命名，除非任务明确要求迁移，否则不要在品牌层面扩大它们。

## 网络规则

- TUN 是 macOS 上受支持的透明代理路径。
- 除非项目明确重新打开该决策，否则 `mihomo.redir_port` 与
  `pf.redirect_tcp_to` 必须保持 inactive。
- 高风险网络改动需要实验室验证，不能只依赖单元测试。
- 结论必须精确说明是否实际运行了 `make lab-test` 或
  `make lab-test-tun`。

## 验证

`make test` 是快速默认门槛，当前等价于 `go test ./...`，也是 CI 级别门槛。

涉及 DHCP、DNS、mihomo 进程或配置生成、pf/NAT、IPv4 forwarding、rollback、
网关生命周期清理、lab 拓扑或 runtime traffic defaults 的改动，
需要用 `make lab-test` 才能宣称真实 host-network 路径被验证。

涉及透明代理的改动，需要用 `make lab-test-tun` 才能宣称 TUN 透明代理路径被
验证。这个门槛会保持客户端无显式代理配置，并要求 HTTPS 流量出现在 mihomo
TUN 路径的日志中。

如果沙箱阻止 Go cache 写入，把 `GOCACHE` 指向 `/private/tmp` 下的路径。

## Agent Wiki 维护规则

`docs/agent-wiki/sources/` 记录稳定来源材料：项目目标、决策和验证契约。

`docs/agent-wiki/wiki/` 是给未来 agent 优先阅读的上下文页面。当某个改动产生
会影响未来 agent 判断的知识时，同步更新这些页面。

只沉淀可复用知识。一次性日志、临时命令输出、未经验证的猜测和普通 TODO 不
应进入 wiki。
