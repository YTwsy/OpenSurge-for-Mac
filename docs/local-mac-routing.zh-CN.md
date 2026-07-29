# Mac 本机流量模式

OpenSurge 提供与 Clash Verge Rev 类似的 **规则 / 全局 / 直连** 切换，但作用域明确限定为
Mac 本机。它不会改变 mihomo 顶层的 `mode: rule`，也不会改变下游设备的规则、设备
selector 或 DHCP/DNS 配置。

## 三种模式

| 模式 | Mac 本机新连接 | 下游设备 |
| --- | --- | --- |
| 规则 | 继续匹配 imported/managed 网关规则 | 继续匹配网关规则或各自的设备策略 |
| 全局 | TCP 使用“本机全局出口” | 不变 |
| 直连 | 直接使用 `DIRECT` | 不变 |

“全局”并不等于修改所有设备的全局规则。它只是把符合本机身份的连接送入一个专用的
隐藏 selector。选择的出口不支持 UDP，或者 OpenSurge 无法确认其 UDP 能力时，本机 UDP
会使用 `REJECT`，避免静默落回网关规则或直连。

局域网、回环、链路本地、CGNAT 和组播目标始终在模式规则前保持直连，避免选择远端
代理后失去本机与 LAN 管理路径。

## 哪些流量属于“本机”

OpenSurge 同时约束 **入口类型** 和 **源地址**：

- TUN 入口中源地址为 mihomo 本机 TUN 身份 `198.18.0.1` 的连接；
- 从 `127.0.0.0/8` 或网关 Mac LAN IPv4 进入 mihomo mixed-port 的本机显式代理连接。

下游连接使用它们自己的 LAN IPv4，不会命中这些本机规则。它们随后继续进入设备
`SRC-IP-CIDR` 覆盖和 imported/managed 网关规则。

因此，Web GUI 中的“Mac 本机流量模式”和设备的“跟随网关规则 / 独立设备出口”是两套
正交控制：

- 切换 Mac 模式无需重载，影响新连接；
- 修改设备身份、路由方式或规则仍需保存后重载；
- 切换已经 applied 的设备 selector 仍只影响那台设备。

## TUN 与 macOS 系统代理

OpenSurge 不会替你开启或改写 macOS“系统设置 → 网络 → 代理”。当 TUN 已启用时，
可路由的 Mac 本机 IPv4 流量由 TUN 进入这套模式；没有经过 TUN 的流量不在其作用域。
应用若显式使用 OpenSurge mixed-port，也会进入同一模式。
Web GUI 的连通性检测由本机 Control Service 经 mixed-port 发起，因此也反映当前本机
模式；它不能作为下游设备仍按网关规则运行的证据。

所以这里的“全网”是 **Mac 本机经 OpenSurge 数据面的全局选择**，不是对所有协议、
所有 Network Extension 或下游设备的无条件接管。现有连接不会被强制中断；如需立即
观察新模式，应重新发起连接。

## CLI

```bash
./bin/omg local-routing --config /etc/open-mihomo-gateway/config.yaml

./bin/omg local-routing-set \
  --config /etc/open-mihomo-gateway/config.yaml \
  --mode rule

./bin/omg local-routing-set \
  --config /etc/open-mihomo-gateway/config.yaml \
  --mode global \
  --policy "Proxy"

./bin/omg local-routing-set \
  --config /etc/open-mihomo-gateway/config.yaml \
  --mode direct
```

内部 selector 使用 `open-surge/mac-*` 保留命名空间，并通过 mihomo
`profile.store-selected: true` 跨 mihomo 重启保存选择。普通 `policies`、`providers`、
节点健康与 `policy-select` 不展示或接受这些内部组；应使用专用的 `local-routing`
命令或 Web GUI。
