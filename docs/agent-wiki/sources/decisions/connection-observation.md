# 连接观察与刷新契约

本页维护流量归属、活跃连接快照、速率、界面展示和显式关闭连接的范围。设备登记与路由编译见 [设备策略](device-policy-overlays.md)，本机路由选择见 [本机模式](local-mac-routing-modes.md)。

## 身份与归属

总览使用受认证的 `GET /api/v1/device-traffic`，连接页使用 `GET /api/v1/connections`。
两者复用同一份后端归属、计数器差值和一秒内共享的采样；配置、运行状态、租约或登记
变化会使缓存失效。不要在前端重复解释 raw connections 的设备身份。后端用 DHCP lease、
applied 设备、desired 登记和当前观察到的网关 LAN 源 IPv4 建立清单。只有已进入 applied
bundle 的 compiled devices 可以作为运行中的登记身份；未应用、暂停的 IP-only 登记和
网段外设备保留在清单中，但不能作为归属证据。IPv4 仍按源地址归属，冲突身份不强行合并。
本机身份由
`internal/controlapi/gateway_local.go` 统一判断：系统 TUN 快照额外读取一次 mihomo
`/configs` 的实际 `inet4-address`/`inet6-address`，同时要求 `type=Tun`、
`inboundName=DEFAULT-TUN` 和精确本机源地址。只取接口地址，不取整个 CIDR；下游也
经过 `DEFAULT-TUN`，并可能具有相同 `inboundIP`，所以入口字段不能单独证明本机来源。
回环/网关 Mac 源地址仍作为本机身份，但 `opensurge-ipv6` listener 和
`inboundUser=device:…` 优先排除。累计流量、速率和本机关闭连接操作复用这个判断。

不能用 `process/processPath` 或共享源 IP 推断本机身份。mihomo 默认 `strict` 只在
规则需要时查询进程；订阅有无 `PROCESS-NAME`、规则顺序、进程查询失败都不能改变
流量归属。不能通过强制 `find-process-mode: always` 代替身份修复。IPv6 地址必须来自
实际运行状态，不能按 desired `auto` 或旧快照同时猜测 fake-AAAA 与显式 TUN 两种身份。
读取 TUN 身份失败时，流量 API 返回 `connection_error` 并保留可确认的清单；本机关闭
连接接口返回 `local_identity_unavailable`，不部分执行。纯显式代理快照不额外读 `/configs`。

本机连接聚合到独立 `gateway_local`，不能把 Mac 放进 `devices`、下游设备数量或策略身份模型。GUI 在“活跃
设备”中固定把“本机 Mac”显示为第一行，并根据实际 connection type 显示 TUN、显式代理
或两者；网关停止时显示网关未运行。

没有 DHCP/静态身份但能确认网关 LAN 源 IPv4 的行使用 `observed_traffic`，其连接数进入
`unidentified_device_connections`，GUI 称为“待识别设备连接”。地址缺失、网段外且没有
本机证据等剩余连接进入 `unclassified_connections`，可在连接页的“无法归属”中查看。
`unmatched_connections` 是旧客户端兼容字段，当前 GUI 不再用它解释来源身份。
`identity_source` 必须区分 `gateway_local`、`dhcp_lease`、`registered_static` 与
`observed_traffic`；无法归属的独立汇总行使用 `unclassified`。主出口按累计字节最多的完整 chain 选择。

下游 IPv6 归属必须同时满足 `type=Tun`、`inboundName=opensurge-ipv6`、合法 IPv6
源地址和 `inboundUser=device:<id>`，且 ID 对应当前 LAN 中带 MAC 的有效 applied 设备。
同一设备的 IPv4、多条 IPv6 隐私地址聚合到 `device:<id>`；未知 ID、其他 listener、
缺少身份或 MAC 冲突保留为无法归属，不能按 IPv6 前缀或进程猜测。本机精确 TUN 身份
仍优先排除下游身份。`connections[].owner_key`、`source_family` 和速率由后端统一给出；
`gateway_totals = gateway_local + totals + unclassified`，计数、会话字节和速率均可核对。
这里的网关总量也只表示当前活跃连接，不等同于网卡累计字节或 mihomo 历史总量。

## 快照与界面

连接页是独立一级导航，设备页继续负责配置，诊断页负责操作/Provider/日志。总览与设备页
可按 owner 深链接到连接页。筛选、排序、选择设备和滚动位置在切页返回时保留；每页最多
显示 50 条连接。暂停会取消进行中的请求并固定画面，恢复后重新采样；页面隐藏或卸载时
停止有效更新。连接离开快照后仅保留选中项的最后详情，不把它扩展成历史连接日志。
“有租约”“已应用登记”“观察到流量”分别展示，没有活跃连接不表示设备离线。
主路由旁路的 IPv4 显示不在统计范围内。网络错误保留上次快照并标注过时；核心不可用
仍返回清单并隐藏统计；租约/登记读取错误单独以 `inventory_error` 标明清单可能不完整。

这是 `active_sessions` 快照，不是持久化历史。mihomo 不可用时仍返回本机、lease 与
applied 静态设备 inventory，并通过 `connection_error` 明确统计不可用。实时 bytes/s
由 Control Service 在内存中按 connection ID 比较相邻采样得到；首次、新连接、长采样
间隔和错误恢复先建立基线。GUI 每 2 秒读取一次，只保存最近 60 秒趋势；总览和点击本机/
设备后展开的趋势卡复用同一图表语义，不能把这段内存数据描述为今日/月度历史。图表使用
平滑曲线；速率数字必须在新采样到达时立即更新，只有曲线在相邻前端采样之间使用约
700 ms 的缓出插值；系统请求减少动态效果时曲线也必须直接采用新值。网关总趋势保持
紧凑并只留低对比参考线。宽屏设备详情不能参与设备列表行高
计算，否则展开/收起会改变文档高度并造成页面底部滚动跳动；窄屏纵向展开则需要显式
高度过渡。

主出口的累计字节相同时，再按连接数和名称稳定决胜。采样间隔超过 15 秒需重建基线；首次、新连接与错误恢复的基线速率为零，界面仍须区分“尚未形成速率”与有效零速率。

## 显式连接刷新

运行态实际切换 Mac 本机模式/固定出口或已应用设备的 selector 后，Web GUI 在全局
右下角保留作用域明确的连接刷新提示；切页不会丢失，同一对象连续切换只保留最新选择。
它默认不打断连接，必须由用户显式调用
`POST /api/v1/local-routing/connections/refresh` 或
`POST /api/v1/devices/<id>/connections/refresh`。刷新按最新快照关闭该作用域内当前由
OpenSurge 管理的全部匹配连接，并不只筛选仍使用旧出口的连接，也不保证客户端自动重连。
因此文案必须提示下载、通话等可能中断。停止态预选和重复选择不产生刷新邀请；卡片内原有的手动刷新入口继续保留。

策略与节点健康页切换普通共享 selector 后，同样提供可跨页面保留的刷新提示，使用
`POST /api/v1/policies/<group>/connections/refresh`。后端校验该组存在于运行中核心的
可见策略组中，再读取最新 connections，以 `chains` 中完整组名的精确匹配选择连接。
该范围包含实际经过此组的 Mac、跟随网关规则的设备，以及通过嵌套策略引用该组的设备；
它与设备登记模式、IPv4/IPv6 或当前叶子节点无关。使用同一节点但未经过此组的连接不得
关闭，也不能通过逐台刷新设备来扩大到其无关连接。刷新仍包含已走新出口的匹配连接，
并由客户端决定何时重连。组名在 URL 中编码，内部 `open-surge/mac-*` 组继续走原有本机
接口。切换本机专用全局出口不会改变下游规则，也不扩大其本机刷新范围。

## 验证

控制面单元测试验证身份排除与精确作用域；真实设备和共享组关闭连接使用 [每设备策略门槛](../validation/test-gates.md#每设备策略门槛)，本机 TUN 身份使用 [本机隔离门槛](../validation/test-gates.md#Mac-本机模式隔离门槛)。IPv6 packet 归属使用对应拓扑门槛。
