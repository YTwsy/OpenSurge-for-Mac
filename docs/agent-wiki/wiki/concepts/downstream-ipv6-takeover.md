# 下游 IPv6 接管

下游 IPv6 接管负责把设备的 IPv6 流量纳入 OpenSurge 的网关与设备策略。
这条实验性路径连接链路接入、设备身份和共享代理引擎，与 Mac 本机 IPv6 路径相互区分。

```mermaid
flowchart LR
    L["下游 IPv6 链路"] --- I["设备身份"]
    I --- P["设备策略"]
    P --- E["共享代理引擎"]
```

地址获取、默认路由、DNS 答案和公网出口能力分别承担不同作用。
共享二层网络还依赖与原路由器的协作；设备进入接管路径的条件决定策略能覆盖的范围。

- 查询拓扑前提、地址、packet path、协议与生命周期 → [IPv6 契约](../../sources/decisions/downstream-ipv6-takeover.md)。
- 查询身份对应的连接归属 → [连接观察](../../sources/decisions/connection-observation.md)。
- 查询验证方法与已知范围 → [IPv6 门槛](../../sources/validation/test-gates.md#下游-IPv6-门槛) / [证据入口](../../sources/validation/evidence-map.md)。
