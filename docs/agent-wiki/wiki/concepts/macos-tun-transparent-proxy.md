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

## 验证

透明代理相关变更使用 `make lab-test-tun`。

该 gate 会保持客户端没有显式代理配置，并证明无显式代理的 HTTPS 请求通过
mihomo TUN 路径出现。当前脚本的直接信号包括客户端 helper 的 transparent
测试、`mihomo.log` 中的 `--> <host>:443`，以及成功时的
`transparent TUN log observed for <host>:443` 输出。

除非这个 gate 实际运行过，否则不要宣称 TUN lab coverage。
