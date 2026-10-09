# 已知验证范围与证据入口

本页只维护影响能力判断的摘要、原记录链接与当前适用性。验证方法见 [test-gates](test-gates.md)；
完整执行记录保留在归档、提交/PR 或独立制品中，不在当前来源累计日期报告。

文档核对点：2026-10-05，`master@405daacba9720731608dba46dd2c7592e62b40d5` 上的
`codex/docs-concepts-sources` 文档工作树。它不是下列报告的被测提交；本次没有重新运行验收。
原文缺少日期、被测提交或环境版本时明确保留缺项，不能用收录或归档时间补齐。

## 物理网络

当前范围回到 [设备策略](../decisions/device-policy-overlays.md)、[IPv6](../decisions/downstream-ipv6-takeover.md)
与 [恢复契约](../decisions/dhcp-recovery.md)；方法见 [物理网络验收](physical-network.md)。

| 具体路径 | 最近已知结果 | 被测日期、版本与环境 | 原执行记录 | 当前适用性与未覆盖项 |
| --- | --- | --- | --- | --- |
| 独立下游 LAN 的 DHCP/DNS、NAT、显式代理、TUN 与受控出口 | 记录了一次成功及停止清理 | 2026-07-06；Mac + Ethernet/AP + Pixel，提交与系统版本未记载 | [物理手机 smoke](../../tasks/finished_archived/real-device-smoke.md#当前验证进度) | 只对应原拓扑与受控上游；不证明当前 master、完整订阅或真实远端出口 |
| 同 LAN 下游 Mac 的 IPv6 TCP、MAC/InUser 与 selector | 强制 IPv6 HTTPS 和浏览器 TCP 成功；同窗口 UDP/443 仍走 IPv4 | 2026-08-13；same_wifi_dhcp、无原生上游 IPv6；提交与 macOS 版本未记载 | [下游 Mac smoke](../../tasks/finished_archived/real-device-smoke.md#2026-08-13-同-LAN-下游-macOS-IPv6-smoke) | 不证明 IPv6 UDP/GUA、竞争 RA 复核、停止撤销、睡眠或其他客户端 |
| same-LAN 的受控 LAN 代理出口 | Pixel 的路由、DNS、Mihomo 与出口 IP 观察一致 | 2026-07-09；Pixel + LAN HTTP 代理；被测提交未记载 | [出口观察](../../tasks/finished_archived/same-lan-tun-smoke.md#proxy-egress-proof) | 不包含 imported provider 切换；后续配置/捕获变化需复验 |
| same-LAN 的 imported provider 切换 | DIRECT 与 egress-proxy 两次浏览器请求及停止清理成功 | 2026-07-10；人工 Android，无 ADB；被测提交未记载 | [人工 Android 记录](../../tasks/finished_archived/same-lan-tun-smoke.md#manual-android-evidence) | 受控本地代理；不代表真实订阅节点、远端出口或 ADB 门槛通过 |
| 同 Wi-Fi DHCP 接管与出口切换 | 单手机手工租约/DNS/出口成功，runner 停止清理 | 2026-07-11；专用 Wi-Fi、Android、静态 Mac；被测提交未记载 | [手工手机记录](../../tasks/finished_archived/same-lan-tun-smoke.md#2026-07-11-manual-phone-validation) | 未扩展为完整路由器/Mac/客户端 DHCP 恢复或双设备兼容验收 |
| 每设备策略早期 Virtual Lab | 两 VM 身份、独立 selector、UDP REJECT 与停止清理通过 | 2026-07-11；提交 7b14586，两台 Lima VM | [原 Lab 记录](../../tasks/finished_archived/device-policy-validation.md#2026-07-11-virtual-lab) | artifact 路径按原记录保留，本次未取得；后续 /22、连接刷新等新断言仍需复验 |
| same-WiFi 双设备与完整 DHCP 恢复 | 原记录明确该轮未执行真机门槛 | 原实现落地时，具体日期与被测提交未记载 | [原未运行说明](../../tasks/finished_archived/device-policy-validation.md#原双设备-same-WiFi-门槛的未运行说明) | 保留 Experimental / cooperative IPv4 边界，不能借用虚拟 Lab 或单手机记录 |

## Tailscale

当前身份、目标授权与角色见 [Tailscale 契约](../decisions/tailscale-outbound.md)。

| 具体路径 | 最近已知结果 | 被测日期、版本与环境 | 原执行记录 | 当前适用性与未覆盖项 |
| --- | --- | --- | --- | --- |
| Mac / 下游经托管节点访问远端 SOCKS5 子网服务；下游使用同节点的公网 Exit Node | 三项均曾取得 HTTPS 成功，原文以单次功能可达性验收 | 真实 Mac、下游终端与远端 LAN；日期、提交、系统版本未记载 | [4via6 与 Exit Node 记录](../../tasks/finished_archived/tailscale-4via6-subnet-smoke.md#最终验收状态) | 不证明具体转发节点的独立抓包、UDP/QUIC、国际出口或长期稳定性，也不替代 lab-test-tailscale |

## 桌面与安装

当前行为见 [桌面宿主](../decisions/desktop-host.md) 与 [分发契约](../distribution.md)，检查见
[桌面方法](desktop-smoke.md) 和 [安装验收](desktop-smoke.md#安装验收)。

| 具体路径 | 最近已知结果 | 版本与环境 | 原记录 | 当前适用性与未覆盖项 |
| --- | --- | --- | --- | --- |
| Next 的 v0.3 Wails 开发迁移与主线归并 | 开发已完成，PR #81 已合入 master | master 405daac 是合入基线，不是全部平台的被测版本 | [已完成迁移计划](../../tasks/finished_archived/desktop-migration-v0.3.md) | 历史阶段表不再指导新开发；迁移完成不扩大安装验收范围 |
| PackageKit、登录连续性、授权与 macOS 13 / Intel 原生运行 | 迁移期原文列为仍需完成的真实安装/平台检查 | v0.3 迁移说明；没有对应的完整实机执行报告或被测提交 | [原验收说明](../../tasks/finished_archived/desktop-installation-v0.3.md#验收层级) | 当前仍需相应新证据；构建、脚本替身与 fixture 不补足这些结果 |
| 旧 Swift 面板展开/聚焦修复 | 四种实现未修复实际展开异常，整体回退到 0932641 | codex/release-v0.1.24；日期与完整环境未记载 | [失败与回退记录](../../tasks/finished_archived/swift-menubar-focus-regression.md) | 仅供旧 Swift 维护与回归追溯，不推导 Wails 当前行为 |

## 使用与更新

先确定具体路径再读对应报告。“最近已知通过”不等于当前 checkout 复验；未收录或未检索到
不等于不支持、未实现或未测试。本入口未收录的物理 Wi-Fi 断链、合盖等路径，应按问题继续
定向取证，不能从测试代码或方法存在补成通过。

新结果扩展范围、暴露阻塞、替代旧证据，或实现变化要求复验时，更新相关摘要和领域限制。
普通测试写入提交/PR 或已有任务记录，不为每次执行新增一行。结束的已有记录进入
`tasks/finished_archived/`，先保留重要结果的直接链接，再归档；原报告不随新版本改写。
