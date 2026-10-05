# 历史 Swift 菜单栏维护经验

归档说明：这是原 GUI Concept 中保留的旧宿主维护与失败观察，转存于 2026-10-05。被测日期未记载；`0932641` 是原记录给出的回退点。仅适用于保留的 v0.2.4 `apps/menubar/`，不构成 Wails 宿主要求。当前原生契约见 [desktop host](../../sources/decisions/desktop-host.md)。

旧菜单栏 App 使用纯 AppKit `NSApplication` 生命周期，不声明占位的 SwiftUI `Settings`
Scene；否则这个由系统管理、可恢复的空窗口可能在部分 macOS 环境中被显示。状态面板使用
AppKit `NSStatusItem` + `NSPopover` 承载现有 SwiftUI `MenuContentView`。状态栏图标点击
与用户从 Finder/Launchpad 再次打开
`/Applications/OpenSurge.app` 进入同一个 presenter；后者只确保面板展开，不触发网关
或 Control Service 生命周期动作。每次菜单栏 App 进程启动完成都会主动展开面板，包括
通过“登录时显示”启动；Finder/Launchpad 的 reopen 事件同样确保面板展开。
首次启动时，`NSStatusItem` 的按钮可能尚未附着到可见窗口；此时 AppKit 会忽略
`NSPopover.show`。菜单栏 presenter 必须在状态栏按钮拥有 window 且可见后展开，但不能把
`NSApplication.isActive` 或 popover window 的 `isKeyWindow` 当作展示前置条件：macOS 14
及以上的 cooperative `NSApplication.activate()` 可能拒绝 LSUIElement App 的请求，激活与
`makeKey()` 都只能是展示后的 best-effort 增强。

App 未 active 时，popover 临时使用 `.applicationDefined`，由状态栏按钮、Escape 与全局鼠标
监听负责关闭；App 获得 active 后再切为 `.transient` 并尝试令真实 window 成为 key。
SwiftUI 面板显式使用 active control appearance，状态栏按钮以持久 `state` 表示面板已展示；
状态轮询仅在 indicator 改变时重绘图标，不能抹掉展示状态。流程由
`applicationDidBecomeActive`、popover delegate 与 common run loop 上短时、有界的退避重试
推进，不接入高频 `applicationDidUpdate`。`NSPopover.isShown` 只表示调用过 `show`，还必须
给动画留出 window 创建宽限期并确认真实 window；超时后执行一次非阻塞兜底展示并清除
pending，不能留下永久卡住状态。macOS 13 仅保留兼容的旧 activation fallback。

不要再尝试"把面板 focus 工作推迟到展开动画结束之后"这一类改法。`codex/release-v0.1.24`
上曾连续提交四版实现：无条件 activation fallback、把展开推迟到 reopen 激活、用
`popoverDidShow` 作为动画完成信号、以及用固定 settle 时间窗跳过
`makeKeyAndOrderFront`。它们都没有修复实际报告的展开异常，反而引入了新的问题，已被
整体回退到 `0932641`。若要重开这个方向，先给出可复现的 WindowServer 级证据和真机
验收，不要只依赖单元测试与 `scripts/check-menubar.sh`。

旧 Swift 退出确认使用同步 `NSAlert.runModal()`，用于避开曾观察到的 SwiftUI alert 回调丢失。浏览器打开使用 `NSWorkspace.shared.open`，失败回退 `/usr/bin/open`；一次性 bootstrap URL 不得进入错误或长期日志。
