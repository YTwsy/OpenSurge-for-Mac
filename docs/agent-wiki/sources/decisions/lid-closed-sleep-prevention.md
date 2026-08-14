# 合盖运行临时接管

## 决策

OpenSurge 的“合盖保持运行”是与网关生命周期解耦的临时系统能力：默认关闭、不持久化
用户意图，只在当前 Control Service 运行期间有效。菜单栏与 Web GUI 共享同一状态和 API。

普通 `caffeinate` / IOPM idle assertion 只阻止空闲系统睡眠，不能可靠覆盖 MacBook
合盖触发。为满足明确的合盖需求，生产 Helper 使用：

```sh
/usr/bin/pmset -a disablesleep 1
/usr/bin/pmset -a disablesleep 0
```

这是全局 root 设置，因此不能把它当成一个可任意遗留的子进程。
[Apple 的 IOPM 文档](https://developer.apple.com/documentation/iokit/kiopmassertiontypepreventuseridlesystemsleep)
把普通 assertion 定义为空闲睡眠防护；[Apple Support](https://support.apple.com/en-us/101114)
则明确记录 `pmset -a disablesleep 1` 会禁用所有睡眠功能。

## 所有权与恢复

- Control Service 启用时打开一条到 root Helper 的长期 lease 连接；EOF 即释放。
- Helper 在写入 `SleepDisabled=1` 前创建
  `/Library/Application Support/OpenSurge/runtime/sleep-prevention-owned`。
- marker 使用持久路径，因为 `pmset` 设置可能跨重启保留；`/var/run` 不能作为重启恢复证据。
- Helper 启动时先 reconciliation；有 marker 才写回 0 并删除 marker。
- pkg preinstall 与卸载脚本在 bootout Helper 前执行相同的 marker 门禁恢复。
- 若启用前已经观察到外部 `SleepDisabled=1`，拒绝接管，避免关闭 OpenSurge 时覆盖其他工具。
- 开关关闭、完整退出、Control Service 崩溃、Helper 重启和 Mac 重启都必须最终恢复；
  “只退出菜单栏 App”不停止 Control Service，因此不会释放。

## 不变量

- 不读取或写入网关 runtime state，不启动/停止 DHCP、DNS、Mihomo、PF 或 forwarding。
- 不保留用户开关偏好，服务重新启动时 UI 必须显示关闭。
- PUT 成功后菜单栏与 Web GUI 都立即采用 Control API 返回的 lease 状态；Web SSE 的
  `state` 签名必须包含 `sleep_prevention`，保证任一界面切换后另一界面会触发刷新。
  菜单栏轮询与 Web 定时 overview 仅作为断线或竞态后的收敛兜底，不能各自保存开关状态。
- UI 必须明确提示耗电、发热和包内运行风险。
- 单元测试与静态打包检查不能替代真实 Mac 合盖、进程 kill 和重启验收。
