# 重载就绪超时与回滚后的 DHCP 接管状态一致性

- 状态：待处理；用户于 2026-10-21 要求独立记录，本次不修复。
- 发现版本：安装版 v0.3.2 build39，来源提交 `4ab752b51a758fdd359e83b991e523c2c7370aa5`。
- 环境：macOS 14.8.5 / arm64，现有 `same_wifi_dhcp` 网关，Mihomo `1.19.30-opensurge.1`。
- 来源：[自定义 DNS 有范围实机验收](issue-93-dns-references-and-issue-82-overrides.md#2026-10-21-安装版实机验收部分范围)。
- 范围：独立于 DNS 预览权限和 hosts AAAA 语义，涉及来源应用的 reload/rollback 与控制面恢复状态。

## 已观察到的两个现象

运行中应用自定义 DNS 候选时，一次 reload 已停止旧服务，但新 Mihomo API 未在当前 2 秒
窗口内就绪，返回 `connection refused`。来源应用事务恢复旧配置并成功启动旧网关，错误中
包含 `previous config restored; previous running gateway preserved or restored`。后续复测成功。
尚不能由单次超时判定启动慢的具体原因，也不能直接把延长超时当作完整修复。

旧网关已经恢复运行，控制面的 DHCP 接管记录却从 `gateway_active` 退回
`router_dhcp_disabled_confirmed`。实测核对了运行网关和接管记录后恢复原记录，原始失败保留。
因此，核心就绪超时是触发事件；回滚成功后仍把恢复阶段写成待启动，是另一个需要修复的
状态一致性问题。两者可分别复现和处理。

## 代码线索

- `internal/mihomo/manager.go`：`Manager.Start` 调用 `waitForAPI(pid, 2*time.Second)`；
  API、DNS 与 TUN 就绪检查有不同窗口，需要结合进程日志和启动时间调查。
- `internal/controlapi/configuration_actions.go`：`applyProfile` 在 reload 失败后恢复旧 config，
  runtime 不存在时尝试启动旧网关。必须区分回滚启动成功、失败和尚有残留三种结果。
- `internal/controlapi/server.go`：`recordReloadRecoveryFailure` 根据错误文本中的
  `reload start failed after gateway stop` 标记失败，即使随后已恢复 runtime 仍可重写阶段。
  后续实现应以明确的事务结果和实际状态协调恢复记录，避免单靠错误字符串推导最终网关状态。

## 后续验收要求

1. 用可控的 API 启动延迟验证就绪超时、提前进程退出、成功启动及取消的边界；记录具体原因，
   保留有界超时、进程清理与回滚能力。
2. 覆盖候选启动失败但旧网关恢复成功：来源仍报应用失败，旧 desired/applied 对齐，DHCP
   接管阶段保持与恢复后的健康网关一致，且不得提示重复启动或错误恢复路由器 DHCP。
3. 覆盖旧网关恢复也失败、stop 部分失败、runtime degraded/残留：保留真实恢复要求，不能
   只因 state 文件存在就报告 `gateway_active`，也不能抹掉失败说明。
4. 补齐来源 apply 与显式 reload 的回归；按生命周期和 DHCP 恢复门槛运行对应 host-network
   检查，再在实际 same-WiFi 环境复验。此次记录不代表已运行这些检查。

证据中的原始配置、地址和完整日志仅留在私有临时目录。脱敏结果见原实机记录链接；本任务
不复制真实网络信息或凭据，也不改变原实测版本与未覆盖项。
