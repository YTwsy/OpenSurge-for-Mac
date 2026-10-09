# 透明接入与 TUN

透明代理让流量在客户端没有显式代理设置时进入代理引擎。
OpenSurge 使用 macOS TUN 承担 Mac 本机与 IPv4 下游的透明接入。
实验性的下游 IPv6 使用保留设备身份的独立 packet path，并与同一网关协作。

接入路径、DNS 解析和出口选择是不同职责；引擎可访问也不能单独说明透明路径已经可用。

- 查询支持路径、退役路径、readiness 与冲突判断 → [TUN 决策](../../sources/decisions/tun-mainline.md)。
- 理解另一条设备接入路径 → [下游 IPv6](downstream-ipv6-takeover.md)。
- 查询透明流量需要哪些证据 → [TUN 门槛](../../sources/validation/test-gates.md#透明代理门槛)。
