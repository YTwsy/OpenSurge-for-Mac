# 已知验证范围与证据入口

本页只维护影响能力判断的摘要、原记录链接与当前适用性。验证方法见 [test-gates](test-gates.md)；
执行记录的归属与归档遵循 [Wiki 维护规则](../../README.md#阅读与维护)。
本页不记录执行安排或累计日期报告。

文档核对点：2026-10-05，`master@405daacba9720731608dba46dd2c7592e62b40d5` 上的
`codex/docs-concepts-sources` 文档工作树。它不是下列报告的被测提交；本次没有重新运行验收。
原文缺少日期、被测提交或环境版本时明确保留缺项，不能用收录或归档时间补齐。

## 物理网络

当前范围回到 [设备策略](../decisions/device-policy-overlays.md)、[IPv6](../decisions/downstream-ipv6-takeover.md)
与 [恢复契约](../decisions/dhcp-recovery.md)；方法见 [物理网络验收](physical-network.md)。

| 具体路径 | 最近已知结果 | 被测日期、版本与环境 | 原执行记录 | 当前适用性与未覆盖项 |
| --- | --- | --- | --- | --- |
| 独立下游 LAN 的 DHCP/DNS、NAT、显式代理、TUN 与受控出口 | 记录了一次成功及停止清理 | 2026-07-06；Mac + Ethernet/AP + Pixel，提交与系统版本未记载 | [物理手机 smoke](../../tasks/finished_archived/real-device-smoke.md#当前验证进度) | 只对应原拓扑与受控上游；不证明当前 master、完整订阅或真实远端出口 |
| 同 LAN 下游 Mac 的 IPv6 TCP、MAC/InUser 与 selector | 强制 IPv6 HTTPS 和浏览器 TCP 成功；同窗口 UDP/443 仍走 IPv4 | 2026-08-13；same_wifi_dhcp、无原生上游 IPv6；提交与 macOS 版本未记载 | [下游 Mac smoke](../../tasks/finished_archived/real-device-smoke.md#2026-08-13-同-lan-下游-macos-ipv6-smoke) | 不证明 IPv6 UDP/GUA、竞争 RA 复核、停止撤销、睡眠或其他客户端 |
| same-LAN 的受控 LAN 代理出口 | Pixel 的路由、DNS、Mihomo 与出口 IP 观察一致 | 2026-07-09；Pixel + LAN HTTP 代理；被测提交未记载 | [出口观察](../../tasks/finished_archived/same-lan-tun-smoke.md#proxy-egress-proof) | 不包含 imported provider 切换；后续配置/捕获变化需复验 |
| same-LAN 的 imported provider 切换 | DIRECT 与 egress-proxy 两次浏览器请求及停止清理成功 | 2026-07-10；人工 Android，无 ADB；被测提交未记载 | [人工 Android 记录](../../tasks/finished_archived/same-lan-tun-smoke.md#manual-android-evidence) | 受控本地代理；不代表真实订阅节点、远端出口或 ADB 门槛通过 |
| 同 Wi-Fi DHCP 接管与出口切换 | 单手机手工租约/DNS/出口成功，runner 停止清理 | 2026-07-11；专用 Wi-Fi、Android、静态 Mac；被测提交未记载 | [手工手机记录](../../tasks/finished_archived/same-lan-tun-smoke.md#2026-07-11-manual-phone-validation) | 未扩展为完整路由器/Mac/客户端 DHCP 恢复或双设备兼容验收 |
| 每设备策略早期 Virtual Lab | 两 VM 身份、独立 selector、UDP REJECT 与停止清理通过 | 2026-07-11；提交 7b14586，两台 Lima VM | [原 Lab 记录](../../tasks/finished_archived/device-policy-validation.md#2026-07-11-virtual-lab) | artifact 路径按原记录保留，本次未取得；后续 /22、连接刷新等新断言仍需复验 |
| same-WiFi 双设备与完整 DHCP 恢复 | 原记录明确该轮未执行真机门槛 | 原实现落地时，具体日期与被测提交未记载 | [原未运行说明](../../tasks/finished_archived/device-policy-validation.md#原双设备-same-wifi-门槛的未运行说明) | 保留 Experimental / cooperative IPv4 边界，不能借用虚拟 Lab 或单手机记录 |

## Tailscale

当前身份、目标授权与角色见 [Tailscale 契约](../decisions/tailscale-outbound.md)。

| 具体路径 | 最近已知结果 | 被测日期、版本与环境 | 原执行记录 | 当前适用性与未覆盖项 |
| --- | --- | --- | --- | --- |
| Mac / 下游经托管节点访问远端 SOCKS5 子网服务；下游使用同节点的公网 Exit Node | 三项均曾取得 HTTPS 成功，原文以单次功能可达性验收 | 真实 Mac、下游终端与远端 LAN；日期、提交、系统版本未记载 | [4via6 与 Exit Node 记录](../../tasks/finished_archived/tailscale-4via6-subnet-smoke.md#最终验收状态) | 不证明具体转发节点的独立抓包、UDP/QUIC、国际出口或长期稳定性，也不替代 lab-test-tailscale |

## 桌面与安装

当前行为见 [桌面宿主](../decisions/desktop-host.md) 与 [分发契约](../distribution.md)，检查见
[桌面方法](desktop-smoke.md) 和 [安装验收](desktop-smoke.md#安装验收)。

可选 HTTP 局域网管理新增独立的密码与监听边界。现有桌面或网关历史记录不覆盖这条新
入口；认证单元测试、浏览器 fixture 不能推导物理手机可达、二维码实际扫描或远程启停
成功。宣称这些能力已验收前，须按 [局域网管理门槛](test-gates.md#局域网管理) 补充对应证据。

| 具体路径 | 最近已知结果 | 版本与环境 | 原记录 | 当前适用性与未覆盖项 |
| --- | --- | --- | --- | --- |
| Next 的 v0.3 Wails 开发迁移与主线归并 | 开发已完成，PR #81 已合入 master | master 405daac 是合入基线，不是全部平台的被测版本 | [已完成迁移计划](../../tasks/finished_archived/desktop-migration-v0.3.md) | 历史阶段表不再指导新开发；迁移完成不扩大安装验收范围 |
| PackageKit、登录连续性、授权与 macOS 13 / Intel 原生运行 | 迁移期原文列为仍需完成的真实安装/平台检查 | v0.3 迁移说明；没有对应的完整实机执行报告或被测提交 | [原验收说明](../../tasks/finished_archived/desktop-installation-v0.3.md#验收层级) | 当前仍需相应新证据；构建、脚本替身与 fixture 不补足这些结果 |
| 总览图表渲染开销与切页连续性 | 受控对比中 Graphics and Media 平均 CPU 从 58.95% 降至 1.51%；明暗主题与 5 次快速切页检查通过 | 2026-10-09；macOS 14.8.5 arm64 / 系统 WebView；原实现 405daac，对比相同基线上的图表修复候选，渲染与采样实现现见 4046861 | [问题、方法与汇总结果 #84](https://github.com/YTwsy/OpenSurge-for-Mac/issues/84) | 每组 45 秒、合成 UI fixture、单核 CPU 口径；不证明网关吞吐量、全部 Mac 或完整睡眠唤醒场景；后续渲染改动需复验 |
| 旧 Swift 面板展开/聚焦修复 | 四种实现未修复实际展开异常，整体回退到 0932641 | codex/release-v0.1.24；日期与完整环境未记载 | [失败与回退记录](../../tasks/finished_archived/swift-menubar-focus-regression.md) | 仅供旧 Swift 维护与回归追溯，不推导 Wails 当前行为 |

## 跨重启恢复与配置校验

系统 DNS 统一按独立持久备份恢复：原 UUID 消失则跳过，不迁移到同名新服务；运行期间
外部 DNS 修改也由原备份覆盖。DNS 恢复失败只记录 warning，继续其他网关清理，未恢复的
备份独立于 `state.json` 保留以供后续重试。
行为边界见 [系统 DNS 契约](../decisions/local-system-dns-coordination.md#生命周期与所有权)。
当前方案的全包 Go、独立备份持久化与生命周期回归、前端提醒测试及生产构建已通过。
Lab 门槛在前置检查退出（实验室尚未启动，且 sudo 凭据不可用），尚无该方案的
host-network 通过证据。单元回归也不证明
真实服务重建或系统重启已验收，仍需按
[本机模式门槛](test-gates.md#mac-本机模式隔离门槛) 和实际 UUID 变化场景复验。

后续恢复修复已拆分旧配置读取与新候选验证，prepared 清理错误不再阻止 Helper 监听，
DHCP 恢复跳过无关策略草稿，租约错误独立于生命周期状态；Mihomo 重启不再要求
dnsmasq 二进制或未应用策略文件，明确消失的下游接口不再阻止 IPv6 本地清理。范围见
[Issue #107 修复与校验审查](../../tasks/finished_archived/issue-107-dns-recovery-and-validation-audit.md)。
这些改动已有隔离回归，仍需 host-network/TUN 及对应拓扑的 IPv6 门槛复验；不能把
单元测试或旧 Lab 通过记录扩展为真实接口拔插、客户端撤销和系统重启均已验收。
配置语法/类型损坏、未知进程身份及未建立完整 applied 配置清单的限制仍保留。

## 订阅自身 DNS 与准备态

默认自身 DNS 绑定与准备态私有监听的实现、支持矩阵见
[profile 契约](../decisions/mihomo-profile-overlay.md#dns-reference-composition-and-binding)。
实现与任务已完成归档；自身引用的证据覆盖隔离核心与准备态，正式自身引用全矩阵和完整
交接未由这些结果证明。各轮实际结果与范围见
[归档记录](../../tasks/finished_archived/issue-93-dns-references-and-issue-82-overrides.md#本次开发与验证记录)，
验收方法见 [Host-network 门槛](test-gates.md#host-network-门槛)。

## 文件式自定义 DNS

已接入文件导入、草稿/预览、候选合成和 Mac/下游 DNS 双入口；实现契约见
[自定义 DNS](../decisions/mihomo-profile-overlay.md#file-based-custom-dns)。
已完成 Mac/Android 双入口端到端检查，具体路径、版本和未覆盖项见
[实机记录](../../tasks/finished_archived/issue-93-dns-references-and-issue-82-overrides.md#2026-10-21-安装版实机验收部分范围)。
预览权限修复已通过安装态复验，范围见
[安装态复验记录](../../tasks/finished_archived/issue-93-dns-references-and-issue-82-overrides.md#2026-10-21-build41-替换安装与预览复验)。
按用户确认以这些端到端结果收尾，Virtual Lab 不作为该任务追加完成条件，见
[归档范围](../../tasks/finished_archived/issue-93-dns-references-and-issue-82-overrides.md#2026-10-21-完成与归档)。
来源重载后的恢复状态一致性已修复，通过控制面回归和隔离真实核心验收，见
[重载与恢复记录](../../tasks/finished_archived/reload-readiness-and-recovery-state.md#2026-10-21-实现与验证)；
该修复未替换安装态服务，也未复验真实 DHCP/TUN 数据面。

## Mac 本机 IPv6 开关

[本机 IPv6 契约](../decisions/local-system-dns-coordination.md#独立的-mac-ipv6-路径)
支持独立控制 Mac 系统 TUN 的 IPv6 路径。已有本机开关及其与 AAAA 设置独立性的部分
实机证据，不扩展为下游 IPv6、原生公网 IPv6 或全部应用兼容结论。具体结果与未覆盖项见
[实机任务记录](../../tasks/finished_archived/issue-93-dns-references-and-issue-82-overrides.md#2026-10-21-安装版实机验收部分范围)，
验收方法见 [本机模式门槛](test-gates.md#mac-本机模式隔离门槛)。

## Sniffer 域名识别

总开关默认关闭，配置所有权与规则边界见
[profile 契约](../decisions/mihomo-profile-overlay.md#gateway-owned-fields)。
当前证据覆盖真实核心中 HTTP Host / TLS SNI 对纯 IP 连接的域名规则补充，以及
`override-destination: false` 保留原始目标 IP；可复验的断言见
[隔离核心测试](../../../../internal/mihomo/sniffer_integration_test.go)，
运行方法见 [Sniffer 门槛](test-gates.md#sniffer-real-core)。
这不是 TUN/LAN、QUIC、ECH 或 Google 登录风控的端到端证据；新开关尚需按相应
拓扑补充 Lab 或真实设备验证，不能借用此前仅观察到缺失域名的连接作为开启后的验收。

## Mihomo 内核迁移

当前构建固定为上游 `3f27ef39f8c96bbfbc8b0b9bb10f5c9ef60149d8` 的
`1.19.33-opensurge.1`，保留 OpenSurge packet listener 与无原生 IPv6 时的配置隔离补丁。
已有双架构编译、arm64 隔离真实核心、策略控制及 Unix packet listener 的受控 UDP/身份
检查；这些结果不证明 BPF、DHCP/RA、真实 TUN 或三个下游 IPv6 拓扑。
当前本地测试包按用户选择暂缓网络回归，历史 `1.19.30` 的网络证据不能直接沿用。
运行步骤与覆盖要求见 [test-gates](test-gates.md)，具体执行版本、结果和未覆盖项保留在
本次迁移提交及安装包的 `.build-info.json` 中。

## 使用与更新

先确定具体路径再读对应报告。“最近已知通过”不等于当前 checkout 复验；未收录或未检索到
不等于不支持、未实现或未测试。本入口未收录的物理 Wi-Fi 断链、合盖等路径，应按问题继续
定向取证，不能从测试代码或方法存在补成通过。

新结果扩展范围、暴露阻塞、替代旧证据，或实现变化要求复验时，更新相关摘要和领域限制。
需要留存的执行记录只写入任务记录，不为每次执行新增一行。结束的已有记录进入
`tasks/finished_archived/`，先保留重要结果的直接链接，再归档；原报告不随新版本改写。
