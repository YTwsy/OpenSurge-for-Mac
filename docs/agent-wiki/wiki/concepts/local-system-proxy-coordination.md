# Mac 系统代理协同

系统代理协同为遵循 macOS 代理设置的应用提供接入方式，承担本机兼容职责。
它与透明流量接管、接入后的路由选择分属不同层次，下游设备有自己的接入路径。

这项能力临时接管系统设置，因此设置的原始状态、所有权和归还责任与代理可用性同样重要。

- 查询启用条件、冲突、写入范围与恢复 → [系统代理契约](../../sources/decisions/local-system-proxy-coordination.md)。
- 查询透明接入与本机路由关系 → [TUN](macos-tun-transparent-proxy.md) / [本机流量](local-mac-routing-modes.md)。
- 查询真实扩展冲突的验收要求 → [兼容层验收](../../sources/validation/test-gates.md#透明代理门槛)。
