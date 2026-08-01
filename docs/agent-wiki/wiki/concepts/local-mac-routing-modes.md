# Mac 本机流量模式

稳定决策来源：
`docs/agent-wiki/sources/decisions/local-mac-routing-modes.md`。

## 不能使用顶层 mode

mihomo 顶层 `mode: global` 或 `mode: direct` 会改变同一进程内的所有流量，包括下游
客户端，因此不能作为 Mac 本机开关。OpenSurge 始终渲染 `mode: rule`，再用排在设备
规则之前的 source-scoped 规则识别本机。

## 生成结构

内部隐藏组为：

- `open-surge/mac-global`：imported proxies/groups/providers 或 managed egress；
- `open-surge/mac-mode-tcp`：`PASS`、`DIRECT`、可选 mac-global；
- `open-surge/mac-mode-udp`：`PASS`、`DIRECT`、`REJECT`、可选 mac-global。

规则同时匹配入口和源：

```text
AND,((IN-TYPE,TUN),(SRC-IP-CIDR,198.18.0.1/32),(NETWORK,TCP)),open-surge/mac-mode-tcp
AND,((IN-TYPE,SOCKS/HTTP),(SRC-IP-CIDR,127.0.0.0/8),(NETWORK,TCP)),open-surge/mac-mode-tcp
```

网关 LAN IPv4 也有一组显式代理入口规则。每个入口在分派到模式组前先生成
local/private `DIRECT` 保护。

`PASS/PASS` 表示 Rule，`DIRECT/DIRECT` 表示 Direct。Global 的 TCP 指向 mac-global；
UDP 只有在 live `/proxies` 能确认当前目标支持 UDP 时才指向 mac-global，否则指向
`REJECT`。TCP/UDP/global 三个选择通过专用控制器事务式切换，失败时逆序回滚。

## 控制面边界

- `GET/POST /api/v1/local-routing` 是唯一 GUI 控制入口；
- CLI 使用 `local-routing` 和 `local-routing-set`；
- 普通 policies/providers/proxy-health/overview 与 snapshot 的策略/provider inventory
  隐藏 `open-surge/mac-*`；诊断日志和连接 chain 可保留实际引擎名称作为证据；
- 普通 `policy-select` 拒绝修改内部组；
- imported profile 和 managed upstream proxy 不能占用保留命名空间。

状态来自 mihomo 当前 selector，并依赖 `profile.store-selected: true` 跨重启保存。
切换只影响新连接，不主动关闭已有连接。Web GUI 必须说明 TUN/本机显式代理作用域，
不能把 Mac 本机模式开关描述成系统代理控制。独立的可选兼容层见
[Mac 本机系统代理协同](local-system-proxy-coordination.md)。

## 验证

单元测试覆盖 AST 组合顺序、模式推导、UDP fail-closed、事务回滚、API/CLI 保留边界。
`make policy-control-test` 使用真实 mihomo 验证生成配置和 live selector 控制。
TUN 与下游隔离必须由 `make lab-test-tun-local-routing` 证明；未运行该门槛时不能声称
真实 host-network 路径已验证。
