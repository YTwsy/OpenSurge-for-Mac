# 持久化网关规则覆盖

OpenSurge 只运行一个 mihomo 进程。订阅 profile 的 `rules` section 由
`internal/mihomo/profile.go` 作为输入，但用户补充规则不能写回订阅快照，否则刷新订阅
会覆盖本地修改。

当前的用户规则覆盖层由 `internal/gatewayrules/` 管理，文件位于
`cfg.RuntimePath("gateway-rules.json")`。它保存三组完整 mihomo 规则文本：

- `prepend`：插入订阅规则之前；
- `append`：插入订阅规则之后、OpenSurge 默认规则之前；
- `delete`：按完整文本从订阅规则中精确移除。

`internal/mihomo/device_policy.go` 在生成 managed/imported 两种配置时加载覆盖层。系统
保护规则、Mac 本机 source-scoped routing、设备专属规则仍在用户 prepend 之前，避免普通
自定义规则改变网关身份和生命周期保护。导入 profile 时，原始 YAML AST 仍保留，覆盖层
只修改最终渲染内容，不修改 `data/imported-profile-*.yaml` 快照。

保存入口是 Control API 的 `GET/PUT /api/v1/gateway-rules`，通过 ETag/If-Match 做乐观
并发控制。root helper 先把候选覆盖层复制到临时 runtime，调用完整 mihomo 配置验证，再
原子写入用户文件。规则值只允许单行、无控制字符或 Unicode 行分隔符，并在 managed
profile 中作为 YAML 字符串引用，避免规则中的 YAML 特殊字符改变文本或突破渲染边界；
helper 在以 root 运行 mihomo 校验前还必须复用 start-input 信任检查，
确认二进制、订阅和设备策略文件都位于 root-owned 边界内。规则 PUT、设备策略 PUT 与网关
生命周期及运行中的订阅 apply 共用 Control Service 生命周期互斥锁，不能让彼此的完整配置
校验读取到不同版本的规则或设备策略，也不能让 reload 预校验和随后 start 读取到两份不同
的规则。订阅 apply 和 gateway reload 的临时候选也必须复制这份覆盖层，否则
会出现“保存时/订阅更新时/重载时验证的配置不同”的错误。

Web GUI 的“网关规则”页面是主要操作入口；保存运行中的规则不会自动重载网关，用户必须
显式重载或等待下次启动。自定义规则的目标名必须存在于当前 profile；如果订阅更新删除了
被引用的策略组，完整候选验证会拒绝该订阅或规则变更，直到用户修正规则。
