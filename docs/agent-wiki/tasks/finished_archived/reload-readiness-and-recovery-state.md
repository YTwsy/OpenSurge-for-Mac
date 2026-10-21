# 重载就绪超时与回滚后的 DHCP 接管状态一致性

- 状态：已完成并归档；2026-10-21 完成等待/回滚状态修复及针对性验收，按用户确认不追加 Virtual Lab 或实际 same-WiFi 数据面复验作为本任务完成条件。
- 发现版本：安装版 v0.3.2 build39，来源提交 `4ab752b51a758fdd359e83b991e523c2c7370aa5`。
- 环境：macOS 14.8.5 / arm64，现有 `same_wifi_dhcp` 网关，Mihomo `1.19.30-opensurge.1`。
- 来源：[自定义 DNS 有范围实机验收](issue-93-dns-references-and-issue-82-overrides.md#2026-10-21-安装版实机验收部分范围)。
- 范围：独立于 DNS 预览权限和 hosts AAAA 语义，涉及来源应用的 reload/rollback 与控制面恢复状态。

## 已观察到的两个现象

运行中应用自定义 DNS 候选时，一次 reload 已停止旧服务，但新 Mihomo API 未在当时的 2 秒
窗口内就绪，返回 `connection refused`。来源应用事务恢复旧配置并成功启动旧网关，错误中
包含 `previous config restored; previous running gateway preserved or restored`。后续复测成功。
尚不能由单次超时判定启动慢的具体原因，也不能直接把延长超时当作完整修复。

旧网关已经恢复运行，控制面的 DHCP 接管记录却从 `gateway_active` 退回
`router_dhcp_disabled_confirmed`。实测核对了运行网关和接管记录后恢复原记录，原始失败保留。
因此，核心就绪超时是触发事件；回滚成功后仍把恢复阶段写成待启动，是另一个需要修复的
状态一致性问题。两者可分别复现和处理。

## 发现时的代码线索

- `internal/mihomo/manager.go`：`Manager.Start` 调用 `waitForAPI(pid, 2*time.Second)`；
  API、DNS 与 TUN 就绪检查有不同窗口，需要结合进程日志和启动时间调查。
- `internal/controlapi/configuration_actions.go`：`applyProfile` 在 reload 失败后恢复旧 config，
  runtime 不存在时尝试启动旧网关。必须区分回滚启动成功、失败和尚有残留三种结果。
- `internal/controlapi/server.go`：`recordReloadRecoveryFailure` 根据错误文本中的
  `reload start failed after gateway stop` 标记失败，即使随后已恢复 runtime 仍可重写阶段。
  后续实现应以明确的事务结果和实际状态协调恢复记录，避免单靠错误字符串推导最终网关状态。

## 原验收计划

1. 用可控的 API 启动延迟验证就绪超时、提前进程退出、成功启动及取消的边界；记录具体原因，
   保留有界超时、进程清理与回滚能力。
2. 覆盖候选启动失败但旧网关恢复成功：来源仍报应用失败，旧 desired/applied 对齐，DHCP
   接管阶段保持与恢复后的健康网关一致，且不得提示重复启动或错误恢复路由器 DHCP。
3. 覆盖旧网关恢复也失败、stop 部分失败、runtime degraded/残留：保留真实恢复要求，不能
   只因 state 文件存在就报告 `gateway_active`，也不能抹掉失败说明。
4. 补齐来源 apply 与显式 reload 的回归；按生命周期和 DHCP 恢复门槛运行对应 host-network
   检查，再在实际 same-WiFi 环境复验。此次记录不代表已运行这些检查。

## 2026-10-21 实现与验证

开发基线为 `codex/release-v0.3.2@b3cde9e`，修复提交为 `617b306`；以下检查针对该提交对应的修复内容。
用户确认本任务以控制面、进程等待和隔离真实核心回归完成验收，原计划第 4 项中的
Virtual Lab 与实际 same-WiFi 数据面复验不再作为本次收尾条件。

- API 等待窗口调整为 10 秒；进程、API、DNS、TUN 与 packet listener 等待接入生命周期
  context，错误保留原因、阶段耗时、最后一次探测和明确启动日志。失败清理继续有界执行，
  清理失败时向网关交还 PID，避免丢失残留进程所有权。
- Reload 返回结构化失败阶段，配置事务另返回配置恢复、旧网关启动尝试及最终健康结果；
  这些结果穿过 Helper 失败响应。旧网关恢复成功保留原 DHCP 运行阶段，来源仍报应用失败。
- 确认 stopped 且 state 消失才返回待启动；部分 stop、degraded、interrupted、读状态失败
  均保留真实恢复要求。候选残留即使健康也不冒充旧配置恢复，不删除仍可能被引用的候选。
- 配置回滚启动使用独立 90 秒 context；取消应用后仍尝试恢复已停止的旧网关。恢复说明和
  原始错误在 operation 结束前写入，原客户端验收与显式跳过记录保留。

检查结果：

| 检查 | 结果与范围 |
| --- | --- |
| `make test` | Go 全量通过；在正常主机权限下读取 boot session，未启动真实网关 |
| 定向 `go test -race` | mihomo/gateway/controlapi 等待、reload、来源事务与恢复状态回归通过 |
| 受控启动夹具 | 2.4 秒延迟可成功启动；有界超时、提前退出、取消、成功后请求结束仍存活及失败清理覆盖 |
| `TestStartupReadinessRealCore` | `1.19.30-opensurge.1` / macOS arm64；注入 2.5 秒延迟后约 3.0 秒 ready，停止后 controller 监听释放；仅随机 loopback，无 DNS/TUN/DHCP/PF 接管 |
| DNS/准备态真实核心回归 | `TestDNSReferencesRealCore` 准备态矩阵、`TestPreparedDNSBindRaceRealCore`、`TestPreparedEngineRealCore`、Workspace 真实核心与候选校验通过；正式 1053 分支未开启 |
| 来源与显式 reload | 三种运行阶段覆盖旧网关恢复成功、再次失败、部分 stop、残留/未知状态及候选仍健康的边界；旧 desired/applied 对齐、失败 operation 和 Helper 元数据均有断言 |
| Lab 前提检查 | 曾尝试 `make lab-test` / `make lab-test-tun`，均在 `lab is not up` 前提处退出；无 Lab 网络启动。状态检查显示 Lab stopped，sudo ticket 不可用，正式 1053 已占用；随后按用户要求不继续 Lab |

本次未替换已安装服务，不把源码/隔离核心检查写成安装态 reload 或真实 DHCP/TUN 数据面
复验。原实机单次慢启动的底层原因未由现有证据定位；修复保证有界等待、可诊断失败与
正确的事务恢复状态。长期规则已进入生命周期、DHCP 恢复契约与验证门槛。

证据中的原始配置、地址和完整日志仅留在私有临时目录。脱敏结果见原实机记录链接；本任务
不复制真实网络信息或凭据，也不改变原实测版本与未覆盖项。
