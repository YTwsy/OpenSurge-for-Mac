# Mac 本机系统代理协同

稳定决策来源：
`docs/agent-wiki/sources/decisions/local-system-proxy-coordination.md`。

`local_system_proxy.enabled` 是默认关闭的 TUN 兼容层，不是新的透明代理实现，也不是
DHCP 接管的一部分。它用于 SafeDNS、DNS Proxy、内容过滤或其他 Network Extension
干扰 TUN-only 本机 DNS，而目标应用仍遵循 macOS HTTP/HTTPS 系统代理的场景。

## 配置与写入边界

- 只允许与 `transparent.mode: "tun"` 同时启用；
- 从 `gateway.upstream_interface` 解析 macOS network service；
- HTTP 和 HTTPS 都指向 `127.0.0.1:<mihomo.mixed_port>`；
- 不写 SOCKS、PAC、自动发现或 bypass domains；
- 已有 active HTTP/HTTPS、PAC、自动发现或认证代理时拒绝启动，不覆盖用户配置。

## 生命周期

启动在任何 host-network 修改前读取并持久化原状态，等 mihomo/TUN、dnsmasq、PF 和
forwarding 都 ready 后才启用 HTTP/HTTPS 代理。Stop 必须先恢复系统代理，再停止
mihomo；start rollback 和 `restart-mihomo` 失败也遵循同一顺序。恢复失败时保留 runtime
state，并避免主动停止仍可承接代理连接的服务，让后续 stop 可以重试恢复。

## 证明边界

单元测试可以证明 `networksetup` 解析、窄写入范围、生命周期顺序、回滚和 state 保留。
开关保持关闭的 `make lab-test-tun` 只能证明 TUN 回归，不证明这个兼容层解决了真实
Network Extension 冲突。要关闭 Issue #16 的产品级验证边界，需要在冲突扩展实际启用
的 Mac 上记录：TUN-only 失败、协同开关启动成功、目标应用流量成功、stop 后原代理状态
恢复。
