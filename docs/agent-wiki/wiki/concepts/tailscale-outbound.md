# Tailscale 出站

OpenSurge 的托管 Tailscale 节点为明确的流量提供出站能力。
本机 Tailscale App 可以提供发现线索；托管节点拥有独立的身份、授权和状态。

Tailnet 目标访问与公网 Exit Node 是不同角色：前者关联目标和来源授权，
后者提供公网出口选择。发现结果、配置意图和实际可达性也具有不同含义。
OpenSurge 在这个模型中承担出站方的职责。

- 查询身份、目标与来源授权、路由冲突和 Exit Node → [Tailscale 契约](../../sources/decisions/tailscale-outbound.md)。
- 查询远端子网与公网出口的证据边界 → [4via6 smoke](../../sources/validation/evidence-map.md#tailscale)。
- 查询托管身份与未授权路径的验证 → [Tailscale Lab](../../sources/validation/test-gates.md#Tailscale-出站门槛)。
