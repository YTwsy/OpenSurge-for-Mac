# Issue #107：DNS 恢复修复与生命周期校验审查

日期：2026-10-31。基线：`master@e450678`（v0.3.2 合入后）。
工作分支：`codex/fix-dns-recovery-107`。

本次范围：修复 #107 的 DNS 备份恢复与生命周期阻断并补回归；审查启动、停止、重载、引擎重启、系统重启、
prepared engine、DHCP 恢复、热拔插、配置修复、状态读取、订阅与 Tailscale 的校验。
DNS 修复已单独提交为 `0d9a148`；第 1–6 项随后获准独立推进，实施范围见文末。
下文“确认”指代码调用链或隔离测试成立，不代表已复现真实主机事故或发生频率。

## #107 的引入与实际失败链

- 首次引入：`cdb28102b78ad84dc3c9ed402106f2f47599494b`，
  [PR #51](https://github.com/YTwsy/OpenSurge-for-Mac/pull/51)，首次正式发布 v0.2.4。
- `system_dns.go` 在 v0.2.4、v0.3.0、v0.3.1、v0.3.2 的 blob 相同。
- `Start` 遇旧 state 就要求 Stop；跨 boot Stop 先恢复 DNS，旧 UUID 无法确认即失败，
  state 因而不能清除，下一次 Start 继续拒绝。这是清理阻断，不是 mihomo 本身启动失败。
- `snapshot.Servers` 是接管前原静态 DNS，空列表代表自动；托管值是单元素
  `[114.114.114.114]`。不能把快照内列表当成当前接管值。

## 本次修复边界

统一使用按备份恢复的 `SystemDNS.Restore`，不再区分普通恢复与跨 boot 专用恢复。
原 DNS 按 service UUID 保存到独立的 `runtime/system-dns-backups.json`，每次
Enable 前先原子持久化；恢复失败的备份不随 `state.json` 清理丢失。
下一次 Prepare 遇到同 UUID 的待恢复备份时复用原值，避免把上次接管值重新备份。
已成功持久化的失败备份可供后续重试；若旧版仅有 state 的备份迁移落盘与 DNS 恢复同时
失败，则只记录警告继续清理，不能承诺磁盘故障下仍保存原值。

| 当前观察 | 处理结果 |
| --- | --- |
| 原 UUID 仍存在，当前 DNS 为接管值、原值或外部修改 | 直接写回原备份并读回验证；空备份恢复自动 DNS |
| 同 UUID 服务改名或接口编号变化 | 使用该 UUID 的当前服务名，不要求旧接口编号仍一致 |
| 原 UUID 消失，包括已有同名新 UUID 服务 | 正常跳过，不扫描同名候选、不把旧备份迁移到新服务 |
| 恢复时读取、设置或读回失败 | warning，独立备份留待重试；继续其他清理，不以该错误保留全部网关 state |
| 无 `state.json` 但有未恢复 DNS 备份 | Stop 继续尝试恢复独立备份 |
| 引擎重启前恢复 DNS 失败 | warning 后继续重启；原服务未确认消失时，新核心就绪后仍尝试接管 |
| 初次 Prepare 读取失败、Enable 前备份保存失败、Enable 设置/读回失败 | 不能宣称接管就绪；启动失败并走完整 rollback，或返回实际重接管错误 |

独立备份与旧 state 内携带的 DNS 快照共同支持恢复。Stop、rollback 和 interrupted
清理在其他清理成功后可移除 `state.json`；未恢复备份仍供后续 Stop 重试。
此处非阻断的是 DNS 恢复错误，不是把所有 DNS 接管步骤改为 best effort。
其他资源的生命周期、旧 boot PID/PF/forwarding 隔离与系统代理恢复规则不因此放宽。

## 网络资料：UUID、网线与 USB 口

1. Apple 将 NetworkServices 及其独立 ID 放在持久配置中；拔以太网线被描述为动态状态更新。
   [System Configuration 架构](https://developer.apple.com/library/archive/documentation/Networking/Conceptual/SystemConfigFrameworks/SC_Components/SC_Components.html)。
2. 创建网络服务会在 NetworkServices 下分配唯一子项，再从路径取得 service ID。
   [SCNetworkServiceCreate 源码](https://github.com/apple-oss-distributions/configd/blob/main/SystemConfiguration.fproj/SCNetworkService.c#L835)。
3. 接口命名会根据类型、接口信息、MAC 历史等尝试复用编号；存在 known-MAC 的复用分支。
   同一 USB 网卡换口不必然创建新 `enX`，更不能直接推导服务 UUID 必然变化。
   [接口匹配](https://github.com/apple-oss-distributions/configd/blob/main/Plugins/InterfaceNamer/ifnamer.c#L2619)、
   [已知 MAC 复用](https://github.com/apple-oss-distributions/configd/blob/main/Plugins/InterfaceNamer/ifnamer.c#L3544)。
4. 网络服务可以独立创建，网络 Location 也可切换/重置设置。
   [Apple 新建服务](https://support.apple.com/en-gb/guide/mac-help/mchlp1176/mac)、
   [Apple 网络位置](https://support.apple.com/en-ca/105129)。

这些资料支持“服务重建可获得新 ID”，不证明异常关机会自动重建报告者的服务。
换 USB 网卡、扩展坞或驱动重新识别值得查证；单纯拔插网线不直接支持该推断。
#107 失效快照属于上游 `en0`，下游是 `en10`；只移动下游 USB 网卡不能直接解释上游
UUID 变化。需要对照操作前后的 service UUID、BSD interface、硬件 MAC 和网络 Location。
公开源码用于机制分析，不视为报告者 macOS build 的逐行实现证明。

## 已确认的恢复阻断与建议优先级

本节是 DNS 修复时的审查基线；后续改动结果见文末，不作为未修复问题清单。

### P1：恢复/修复入口先要求旧 desired 配置完全有效

`config.LoadRuntime` 只跳过 desired device policy，仍运行其他配置语义校验
（`internal/config/loader.go:64`）。`StopConfig` 锁前和锁内都依赖它
（`internal/gateway/lifecycle_lock.go:96`）。

配置 GET/PUT 在读取新请求之前调用 `config.Load(old)`
（`internal/controlapi/server.go:383`）；配置事务也在加载新字段前校验旧配置
（`internal/controlapi/configuration_actions.go:579,608,618`）。
例如旧 LAN 与 DHCP pool 不匹配，提交正确替代配置仍被旧值拒绝，表单 GET 也失败。

Helper 又在监听 socket 前无条件运行 prepared 清理，其内部先 LoadRuntime
（`internal/controlapi/actions.go:263,564`）。即使根本没有 prepared state，旧配置语义
错误也能使 Helper 整体退出，放大为特权恢复功能不可用。

建议：拆分可信路径/恢复目标的最小读取、旧配置诊断、候选部署完整校验。恢复操作不能
以整份新启动配置有效为前提；保留 root-owned、安装根边界、生命周期锁、revision 及
候选部署校验。不能通过简单跳过所有 Validate 把不可信 runtime 路径带进 root 操作。

### P1：prepared journal 不完整阻止 Helper 提供全部服务

`internal/mihomo/prepared.go:242` 先持久化启动意图，再创建进程；中途崩溃可留下没有
PID/fingerprint 的同 boot 记录。`LoadPrepared` 拒绝损坏 JSON/缺 boot 身份（405–410），
停止又拒绝缺进程身份（469–471）。配合 Helper 监听前清理，造成全部接口退出。

拒绝杀未知进程正确；阻断范围过大。建议保持 Helper 诊断和不竞争 cache/tsnet 的恢复
动作可用，只限制有资源冲突风险的动作。睡眠 marker 恢复失败已采用后台重试且继续监听
（`internal/controlapi/actions.go:260`），可作为故障隔离参考。

### P2：DHCP 恢复被无关策略草稿阻断

`loadHelperConfig` 只给 Stop、restart-mihomo、workspace、preview 跳过 desired policy；
`network-set-dhcp` 和 `dhcp-probe` 仍完整 Load（`internal/controlapi/actions.go:547`）。
策略草稿损坏/删除会连带阻断 OFFER 探测、人工确认后的 DHCP 恢复
（`internal/controlapi/server.go:1478,1515,1565`）。应按恢复动作读取最小必要配置，继续
验证真实操作对象和特权路径。现有“人工跳过 OFFER”不能绕过这个无关前置条件。

### P2：附属租约错误遮蔽核心状态与恢复按钮

`gateway.Status` 在判断 boot 前调用 LoadLeases，任意读取/解析错误就返回空 Status
（`internal/gateway/status.go:48`、`internal/device/scanner.go:37`）。
UI 因状态未知隐藏 interrupted 清理、禁用普通运行控制
（`web/src/pages/NetworkPage.tsx:77,554,574`），自动引擎恢复也不再触发
（`internal/controlapi/mihomo_recovery.go:194`）。直接 Stop 本身不依赖租约。

建议：保持可独立确定的 boot/进程/runtime 状态，设备列表错误单独警告；不能把解析失败
伪装成正常空列表，也不应让它消除 Stop 的可用性。

### P2：mihomo-only 重启检查没有使用的 dnsmasq 和 desired policy 文件

Helper 将 restart-mihomo 与完整启动共用 requireTrustedStartInputs
（`internal/controlapi/actions.go:343,650,664`），要求 dnsmasq 二进制及策略草稿存在。
该动作实际使用已渲染的 Mihomo 配置、不执行 dnsmasq，故无关文件丢失会阻断引擎恢复。
建议拆分 restart 所需的可信输入，保留实际执行文件、applied 配置和真实依赖的信任检查。

### P2：同 boot 网卡消失后，IPv6 清理仍要求接口/link-local 可用

`macosipv6.Manager.RemoveGateway` 在 alias 已随接口消失时仍可能返回查接口错误；
`Withdraw` 要求接口与 link-local 存在（`internal/macosipv6/manager.go:196,212`）。
网关清理把这些都作为失败保留 state（`internal/gateway/manager.go:1040`），即使其他
进程、PF、forwarding 已清理。旧 boot 清理不走这条路径。

建议分别表达“资源确认消失”“链路不在，RA撤销无法送达”“状态读取失败”。前者幂等
成功；RA 未送达应保留客户端路由有效期风险说明；未知读失败保留恢复材料。

### P2：订阅私网防护与项目自身 Fake-IPv6 冲突

`safeDialContext` 用系统 resolver，任一结果 IsPrivate 就拒绝整个请求
（`internal/controlapi/sources.go:90`）。项目 Fake-IPv6 是 ULA，若系统解析返回该地址，
即使同时有 IPv4 也会在拨号前拒绝。导入和刷新都使用这条路径。
隔离测试确认函数行为；尚未在真实系统 resolver/订阅场景复现。

不能删除 SSRF 保护或豁免所有 ULA。用户确认本轮保留现有订阅逻辑；尚无真实订阅失败
证据，不能仅根据函数级合成输入测试就认定需要更换下载路径。

## 需要单独设计的候选与反向风险

| 场景 | 当前事实 | 后续边界 |
| --- | --- | --- |
| 系统代理服务改名/删除/重建 | `system_proxy.go:92` 向旧服务名写设置，快照无 UUID，Interface 未用于写前核对 | 既可能无法恢复，也可能碰到同名替代服务；需要身份与当前值所有权设计，不能忽略错误 |
| restart-mihomo 的原 Profile 丢失/摘要变化 | `gateway/manager.go:760` 明确拒绝，已有测试要求拒绝漂移 | 需要完整 applied manifest 后才能脱离 desired；现在不能单删 digest 校验 |
| Tailscale 已有 identity，Auth Key 文件为空 | `mihomo/tailscale.go:175` 缺文件可放行，空文件却拒绝；隔离测试确认 | 先加强 identity 判据（当前只是目录非空），再让首次注册密钥与日常恢复解耦 |
| Tailscale 授权设备移到当前 LAN 外 | `config/validator.go:537` 要求 active device，现测试明确要求拒绝 | 可讨论让已知 inactive 设备 dormant 且不生成授权规则；完全未知 ID 仍拒绝 |
| 扫描无关接口时恰逢热拔插 | `gateway/manager.go:1147` 任意 Addrs 错误阻断启动 | 仅代码候选，未真机复现；可对确认消失重新枚举，不吞权限/未知错误 |
| DHCP 网络快照的 DNS 读取失败 | `macosnetwork/network.go:81` 忽略读取错误；SetManual 的空 DNS 变成 Empty（116） | 这是保护不足，可能将读失败变成自动 DNS；应保留错误，不能继续放松 |

## 审查过且应保留的校验/已有降级

| 场景 | 判断 |
| --- | --- |
| 新接管时真实接口、LAN地址、重复地址检查 | 是 DHCP/NAT/路由正确性的前提；环境暂未就绪应标为可重试，不应绕过 |
| 旧 boot 的 PID/PF/forwarding、reload/restart-mihomo | 必须区分 boot，不能使用旧 PID 或假设内核状态继承 |
| 同 boot PID 复用、未知进程指纹 | 禁止误杀；只应收窄该不确定性阻断的功能范围 |
| 生命周期锁 | 使用内核 flock；未发现按残留锁文件永久锁死的问题 |
| TUN 路由和 DNS readiness | 主要验证本地路由、controller、socket及受控DNS，不要求公网健康；应保留 |
| DNS 自引用/循环与旧来源 listener metadata 缺失 | 无足够来源信息时不能猜 loopback 端口；需迁移/恢复 raw source |
| mihomo -t | 接管前应保留；可区分语法、超时、文件错误，本轮未证明应绕过核心校验 |
| Tailscale warm-up 离线 | 已 best effort，不是整机生命周期成功条件 |
| 来源库副本缺失 | 已优先使用 root-owned 当前 profile，来源库副本可选 |
| 订阅节点/出口消失 | 已有候选回退/跟随全局，保留用户原始选择 |
| DHCP OFFER 探测失败 | 已有明确人工恢复/保持静态完成分支；真正问题是无关配置前置条件 |
| 自动引擎恢复 unknown/旧boot/单次尝试 | 防止不确定状态无限启停，应保留；辅助状态错误需先隔离 |
| 睡眠 marker 恢复失败 | 已后台重试，不终止 Helper，是较好的恢复隔离方式 |

## 本次验证与未覆盖

- 当前 DNS 操作回归覆盖：新 UUID/服务消失不重绑、当前外部 DNS 仍按原备份恢复、
  自动/手动备份、同 UUID 改名/接口变化、服务查询失败、写入及读回失败。
- `GOCACHE=/private/tmp/opensurge-issue107-gocache go test ./internal/macosnetwork -run 'TestSystemDNS' -count=1`
  通过；同缓存运行 `go test ./internal/macosnetwork -count=1` 通过。
- 当前备份恢复方案的 `GOCACHE=/private/tmp/opensurge-issue107-gocache make test` 在正常
  宿主执行环境通过；沙箱中的已有测试因 macOS boot-session 读取限制失败，代码未为此改动。
  回归覆盖恢复失败继续 Stop/rollback/restart、跨 boot 不碰旧 PID/PF/forwarding、旧状态迁移、
  独立备份保留/重试、同 UUID 不重复备份托管值、新 UUID 不继承旧备份及其他清理错误仍可见。
- `make web-test` 通过：38 个文件、354 项测试；新增停止/重启操作的中英文 DNS 恢复提醒。
  `make web-build` 通过（含 i18n 检查、TypeScript 与生产构建），已更新嵌入资源。
  `git diff --check` 通过。
- 实机只读检查曾取得 scutil 网络服务列表；该观察不证明 UUID 重建或 DNS 恢复。
  没有修改本机 DNS。
- 额外 Go overlay 检查 3 项通过，证明当前旧配置阻断修复、Fake-IPv6 被拒绝、
  Tailscale 缺 key 与空 key 行为不同。它们证明现存行为，不是这些项已经修复。
  临时证据：`/private/tmp/opensurge-validation-audit/`，不依赖它作为长期规范。
- Lab 前置检查受阻：正常宿主环境下 `sudo -n true` 和 `make lab-status` 报需要密码。
  复用主工作区工具 PATH 后分别尝试 `make lab-test` 与 `make lab-test-tun-local-routing`，
  两者均在 `lab is not up; run: make lab-up` 处退出；未启动 VM/网关或改变本机网络。
- 未复现真实 service UUID 重建、真实重启、USB 热插拔、真实订阅下载或 DHCP/IPv6 数据面。
  这些需要独立的环境和操作前后证据；普通 Go 回归不替代它们。

## 第 1–6 项后续实施

基于已提交的 `0d9a148` 独立推进；用户确认第 3 项也纳入，订阅下载保持现状。

| 项目 | 已实施的行为 | 保留的边界 |
| --- | --- | --- |
| 旧配置修复/Stop | `LoadEditable` 读取旧表单值，合并新字段后完整校验；`LoadRecovery` 供 Stop、锁定位和恢复诊断使用 | 语法/类型错误和不可读取路径仍失败；新候选、revision、生命周期锁、Helper 文件与路径信任检查不变 |
| Helper 准备态失败隔离 | prepared reconcile 失败记录日志并继续监听；Stop 尝试其余独立网关资源清理 | 未知进程不发送信号，不丢 prepared journal；有资源冲突的启动继续受资源交接门槛约束 |
| DHCP 探测/恢复 | 跳过无关策略文件和未来部署的语义校验 | 非空接口、配置目标、实际 service/interface、OFFER 与人工恢复阶段仍检查 |
| 租约/状态隔离 | 保留有效租约与确定的生命周期状态，返回 `devices_error`；CLI、菜单栏显示设备观测不完整 | runtime 文件/boot 读取失败仍是未知状态，不伪装成正常空列表或 stopped |
| Mihomo 独立重启 | 信任检查不再要求 dnsmasq 二进制或未应用策略文件 | 当前 `LoadRuntime` 其他语义校验、profile digest、实际核心文件、PID/boot 和 `mihomo -t` 保留；完整 applied manifest 未纳入 |
| 下游接口消失 | 明确接口/alias 消失时本地清理幂等；缺接口、link-local 或链路不可达时 RA 撤销独立 warning | 未知/权限/实际删除失败仍报错；不承诺客户端立即撤销旧路由、前缀和 DNS |

配置仅保存拓扑字段时不要求先停止 prepared core；该入口不修改 runtime/profile/cache
路径，也不启动进程。后续 start 的资源交接检查仍保留。CLI `status`、`logs`、`snapshot`
同样使用恢复读取；doctor 保留部署检查。Control API 无法确定恢复路径时返回明确错误，
不再用空配置查询相对路径后报告 stopped。

本轮验证：

- 宿主环境 `GOCACHE=/private/tmp/opensurge-issue107-gocache make test` 通过。
  覆盖旧配置修正/无效候选拒绝、CLI 诊断、Helper socket 存活、准备态未知身份、
  DHCP 目标限制、核心依赖、坏租约的 none/active/interrupted 状态，以及 IPv6
  缺资源、热拔插竞态、Stop/rollback、未知读取/权限错误和 broker PID 保护。
- `make web-test` 通过：38 个文件、357 项测试。新增设备数据不完整、坏租约下仍可
  interrupted 清理，以及本地停止成功时保留 RA 未撤销提示。
- `make web-build` 通过：i18n、TypeScript 与生产构建完成，嵌入资源已刷新。
- `make desktop-test` 通过，包含 race 检查；菜单栏 DTO 转传 `devices_error`。
- `git diff --check` 通过。订阅实现 `internal/controlapi/sources.go` 未改动。
- `sudo -n true` 仍提示需要密码；`make lab-test` 在实验室未启动的前置条件退出。
  未执行真实 host-network、TUN、对应拓扑的 IPv6 门槛，也未启动 VM/修改宿主网络。
  本轮不证明真实重启、USB 拔插或下游客户端撤销，相关能力仍须复验。

临时检查日志在 `/private/tmp/opensurge-recovery-{go-test,web-test,web-build,desktop-test}.log`。
系统代理、完整 applied manifest、Tailscale 及订阅行为未扩展到本轮实现。

## v0.3.3 通知整合

按发布分支 `46a9bd6`（PR #108）引入的反馈契约，将 DNS 恢复失败与 IPv6 撤销未送达
接入既有全局悬浮失败通知。通知独立于整体操作成功结果，保留到手动关闭；进度卡不再
重复显示这两条告警，operation ID/notice 去重避免轮询、关闭和组件重挂载后重发。
设备数据错误继续作为持续状态显示，不反复弹出操作通知。

整合后 Go 全包通过；Web 39 文件/368 项回归、i18n 1918 条、生产构建通过，生成资源
已重建。打包预检在宿主环境通过；原沙箱运行的临时目录 chmod 受限已通过原样重跑排除。
桌面回归此前通过，PR CI 继续验证两个架构。以上不改变本记录的 Lab/物理网络未验证边界。
