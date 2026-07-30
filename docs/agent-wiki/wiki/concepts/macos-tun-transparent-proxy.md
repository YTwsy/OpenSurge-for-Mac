# macOS TUN 透明代理

当任务涉及透明代理配置、mihomo 渲染、PF 规则、文档、测试或验证
结论时，先读这个页面。

TUN 是 OpenSurge for Mac 在 macOS 上受支持的透明代理路径。透明代理应通过
以下配置启用：

```yaml
transparent:
  mode: "tun"
```

旧的 redir/PF redirect 路线不是当前 active implementation path：

```yaml
mihomo:
  redir_port: 0
pf:
  redirect_tcp_to: 0
```

## 为什么是 TUN

当前 Darwin mihomo build 在运行时报告 redir 不受支持。OpenSurge for Mac 需要
可靠的全屋代理路径，因此项目使用 mihomo TUN 承担透明路由，而不是依赖 inactive
的 `redir-port` 加 PF TCP redirection 行为。

## 实现期望

- `internal/config/validator.go` 拒绝非零 `mihomo.redir_port`。
- `internal/config/validator.go` 拒绝非零 `pf.redirect_tcp_to`。
- `internal/mihomo/config.go` 应保持旧 redir 路径 inactive。
- `internal/pf/template.go` 不应重新引入 `rdr pass` TCP redirect 规则。
- 文档应把 TUN 描述为受支持路径，不要描述成候选或实验路线。

## 启动 readiness 与其他 TUN

存在 `utun` 接口本身不是冲突证据。普通 split-route VPN、Tailscale 非 Exit Node
路径和系统组件都可能保留或创建 utun。macOS 也无法可靠证明 utun 的进程所有权，
因此不要在 start、reload、`restart-mihomo` 或 DHCP 接管计划中根据现有公网路由
猜测冲突。真实启动由下面的 readiness fail closed；只有 mihomo 实际报告添加路由
失败后，才查询该目标的当前接口/网关并补充诊断。

mihomo REST API 可以先于 TUN 初始化对外响应，因此 `/version` 成功不代表透明
路径已经就绪。启动流程必须在有限时间内等待运行时 `/configs` 报告
`tun.enable: true`，同时识别 `Start TUN listening error`。当前启动预算是 10 秒；
失败时先给新进程 3 秒 SIGTERM 清理窗口，再按需 SIGKILL，并进入 gateway
rollback。运行中的 status/overview 每次只读取一次轻量运行时状态；若 `/configs`
暂时不可读，TUN 显示 `unknown` 并附带 warning，但不能据此把仍运行的网关改成
`degraded`。只有明确读取到 `tun.enable: false` 才是失败信号。不要增加独立后台
watchdog，也不要在状态热路径反复执行 route/scutil 扫描。

`tun.enable: false` 的失败语义已经针对项目固定的 mihomo v1.19.27 验证。升级
mihomo 时必须重新核对失败后的 `/configs` 行为并跑真实 TUN Lab，不能把这个语义
当作所有历史版本都具备的通用契约。

当前默认不支持与另一个全局 TUN 同时占有公网路由。DNS resolver 状态与 TUN 路由
所有权是不同信号；不要因为出现 utun scoped/supplemental resolver 就判定 TUN
冲突。

## 验证

透明代理相关变更使用 `make lab-test-tun`。

该 gate 会保持客户端没有显式代理配置，并证明无显式代理的 HTTPS 请求通过
mihomo TUN 路径出现。当前脚本的直接信号包括客户端 helper 的 transparent
测试、`mihomo.log` 中的 `--> <host>:443`，以及成功时的
`transparent TUN log observed for <host>:443` 输出。

如果变更涉及 imported profile provider 或通过 `policy-select` 改变透明 TUN
出口路径，使用 `make lab-test-tun-imported-egress`。这个 gate 使用本地 HTTP
provider 和受控 HTTP CONNECT proxy，证明无显式代理客户端的 TUN 流量可以从
`TunEgress[DIRECT]` 切到 `TunEgress[egress-proxy]`。它仍然不是真实订阅节点或远端
出口 IP 验收。

除非这个 gate 实际运行过，否则不要宣称 TUN lab coverage。
