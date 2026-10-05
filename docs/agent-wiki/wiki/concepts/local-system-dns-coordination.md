# Mac 系统 DNS 与流量接管

系统 DNS 决定 Mac 应用怎样取得解析结果，代理引擎的 DNS 决定接入后的解析策略，
TUN 路由决定流量怎样到达数据面。这三者需要协调，也各有独立的职责。

DNS 的临时接管具有所有权与恢复边界；应用自己的解析通道仍需按实际路径判断。
AAAA 答案、Mac 本机 IPv6 路由和下游 IPv6 接管表达三个不同问题。

- 查询系统 DNS 所有权、私有解析和 host TUN 路由 → [DNS 契约](../../sources/decisions/local-system-dns-coordination.md)。
- 查询本机来源身份 → [本机路由契约](../../sources/decisions/local-mac-routing-modes.md)。
- 查询系统解析、重启与恢复证据要求 → [本机模式门槛](../../sources/validation/test-gates.md#Mac-本机模式隔离门槛)。
