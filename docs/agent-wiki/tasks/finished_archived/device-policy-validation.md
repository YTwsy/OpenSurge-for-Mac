# 每设备策略的历史验证记录

此页转存已有验证 Concept 的结果段落，不是本次新增实验。来源为
`docs/agent-wiki/wiki/concepts/validation-gates.md` 的 `405daac` 文档快照；
归档于 2026-10-05。当前方法与适用性分别见 [验证门槛](../../sources/validation/test-gates.md#每设备策略门槛)
和 [证据入口](../../sources/validation/evidence-map.md#物理网络)。

## 2026-07-11 Virtual Lab

2026-07-11 已在 P1-1..P1-5 修复（commit `7b14586`）后运行此门槛并通过：两个 VM
拿到 `.101`/`.102` 固定租约且 `omg devices` identity 就绪，UDP
`192.168.50.101 -> 1.1.1.1:443` 命中设备 `REJECT` fallback，两设备 selector 独立
切换，设备级域名 `REJECT` 生效，stop 后 `state.json` 清除。artifacts 在
`artifacts/lab/20260711-194621`。ARP/ICMP reservation 冲突探测只在 `same_wifi_dhcp`
模式激活，此 lab（`same_lan`/tun）未在运行时覆盖该路径，由单元测试覆盖。

原 artifact 位置按原文保留，本次未重新获取。当前门槛包含后续扩展，旧 fixture 的通过不能替代复验。

## 原双设备 same-WiFi 门槛的未运行说明

以下是同一来源中的历史状态，原文未另记被测提交或日期。该门槛涉及两个真实客户端、主动 DHCP OFFER 与完整恢复；保留原限制：

截至本实现落地时该 gate 尚未在本轮真机运行；因此 same-WiFi per-device 只能标记为
Experimental / cooperative IPv4，不能借用 virtual lab 的通过记录宣称已验收。
