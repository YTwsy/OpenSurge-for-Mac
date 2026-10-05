# Agent Wiki 索引

已知问题可直接进入右侧来源或代码。Concept 用于理解概念关系；具体规则以对应 source
为主要维护位置。使用文档见 [用户目录](../../user/README.md)。

## 领域地图与当前契约

| 主题 | 理解关系 | 查具体规则 |
| --- | --- | --- |
| 网关、资源所有权与恢复 | [网关生命周期](concepts/gateway-lifecycle.md) | [生命周期](../sources/decisions/gateway-lifecycle.md) / [DHCP 恢复](../sources/decisions/dhcp-recovery.md) |
| 共享 GUI 与业务状态 | [GUI 控制面](concepts/gui-control-plane.md) | [GUI](../sources/decisions/gui-control-plane.md) / [API 与配置应用](../sources/control-api.md) / [连接观察](../sources/decisions/connection-observation.md) |
| 原生桌面与后台服务 | [桌面宿主](concepts/desktop-host.md) | [宿主契约](../sources/decisions/desktop-host.md) / [分发安装](../sources/distribution.md) |
| 透明流量接入 | [TUN](concepts/macos-tun-transparent-proxy.md) | [TUN 决策与 readiness](../sources/decisions/tun-mainline.md) |
| 下游 IPv6 与设备身份 | [IPv6 接管](concepts/downstream-ipv6-takeover.md) | [实验性 IPv6 契约](../sources/decisions/downstream-ipv6-takeover.md) |
| 来源、草稿与引擎配置 | [配置合成](concepts/mihomo-profile-overlay.md) | [profile overlay 与准备态](../sources/decisions/mihomo-profile-overlay.md) |
| 设备、匹配与出口 | [设备策略](concepts/device-policy-overlays.md) | [身份与编译](../sources/decisions/device-policy-overlays.md) |
| 本机与下游作用域 | [本机流量](concepts/local-mac-routing-modes.md) | [本机模式契约](../sources/decisions/local-mac-routing-modes.md) |
| 系统 DNS、网关解析与 fake IP | [DNS 协同](concepts/local-system-dns-coordination.md) | [DNS 所有权与路由](../sources/decisions/local-system-dns-coordination.md) / [引擎 DNS 配置](../sources/decisions/mihomo-profile-overlay.md#Imported-DNS-policy) |
| 应用系统代理接入 | [系统代理协同](concepts/local-system-proxy-coordination.md) | [兼容层与恢复](../sources/decisions/local-system-proxy-coordination.md) |
| Tailnet 与公网出口 | [Tailscale 出站](concepts/tailscale-outbound.md) | [身份、授权与 Exit Node](../sources/decisions/tailscale-outbound.md) |
| 临时合盖运行 | — | [Helper lease 与睡眠所有权](../sources/decisions/lid-closed-sleep-prevention.md) |

## 开发、验证与证据

- 项目定位与实现入口：[项目简报](../sources/project-brief.md)。
- 本地调试、构建和资源更新：[开发说明](../sources/development.md)。
- PKG 构建、版本、签名和产物：[构建与发布](../sources/releasing.md)。
- 理解证明范围：[验证概念](concepts/validation-gates.md)；选择具体检查：[test-gates](../sources/validation/test-gates.md)。
- Lab 环境或清理故障：[lab-gates](../sources/validation/lab-gates.md)。
- 原生交互与安装检查：[desktop smoke](../sources/validation/desktop-smoke.md)；物理客户端与恢复：[物理网络](../sources/validation/physical-network.md)。
- 判断支持、通过或阻塞：[证据入口](../sources/validation/evidence-map.md)，再读对应日期记录。

历史仅在验收判断、回归或版本追溯时沿证据入口定向读取；默认检索遵守 `.ignore`，不展开归档清单。

许可证以仓库根目录 [LICENSE](../../../LICENSE) 和 [THIRD_PARTY_NOTICES](../../../THIRD_PARTY_NOTICES.md) 为准。
