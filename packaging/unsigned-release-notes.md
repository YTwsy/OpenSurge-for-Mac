[简体中文](#v030-主要变化相对-v024) · [English](#english)

> **v0.3 series codename: Verdilion**

### v0.3.0 主要变化（相对 v0.2.4）

v0.3.0 正式发布，带来独立的 macOS 桌面 App、重新整理的菜单栏状态面板和设备工作台。本次汇总六个候选版本的桌面、设备管理与状态显示改进。

- **一致且准确的网关状态**：菜单栏、侧栏与总览区分启动、重载、停止、恢复和回滚进度；配置待应用与诊断提醒单独显示，实际服务故障给出具体原因。操作边界重新确认状态，迟到响应和遗留记录不会覆盖当前结果。
- **完整的桌面操作入口**：OpenSurge 现在直接在独立桌面窗口中展示总览、网络设置、设备、连接、策略和诊断。窗口支持原生菜单、快捷键、文件选择与保存；隐藏后重新打开会保留当前页面和草稿。菜单栏也保留「在浏览器中打开」入口。
- **更实用的菜单栏状态面板**：集中展示上传下载速率、近 60 秒趋势、本机出口和活跃设备。网络状态、本机出口与具体设备可在面板内展开；窗口随内容调整大小，并提供展开过渡。网关停止时收起流量曲线，显示紧凑提示；桌面窗口隐藏后，菜单栏仍独立采样流量。
- **统一的设置入口与外观**：通过「OpenSurge → 设置…」、`⌘,` 或侧栏快捷设置进入独立设置窗口，管理语言、外观、登录时显示、临时合盖保持运行与更新，并提供退出和卸载入口。设置顶部新增「GitHub 仓库」和「文档」链接，点击后在系统浏览器中打开。侧栏底部保留扁平状态样式，齿轮可展开快捷设置；标题栏、列表、滚动条与文字选择统一使用 OpenSurge 的界面风格。
- **更连贯的窗口体验**：Dock 图标跟随桌面和设置窗口的显示状态；启动加载阶段展示浅色渐变与 OpenSurge 图标。App 在后台运行且窗口已关闭时，再次点击图标会重新显示主窗口，修复设置窗口被一同打开、菜单栏面板闪现的问题。同步改善退出和卸载确认窗口的前台显示，以及与后台服务断开后的重连和状态保留。
- **登录项可以重新注册**：修复更新 App 后「登录时显示」开关被禁用、误提示需要打开已安装 App 的问题。当前版本的登录项缺失时，可在设置中重新开启；等待 macOS 批准或注册失败会显示真实状态和错误，不会把点击开关当作注册成功。
- **节点检测逐项显示结果**：策略页每完成一个节点的检测就更新该节点结果，无需等待整批完成。检测节点期间保持实际网关状态，不再误报“正在更新网关”。同步改善连通性页面文字对比度、策略页控件对齐、菜单栏设备跳转，以及诊断日志的局部横向滚动。
- **设备工作台与出口操作**：左侧搜索和选择设备，右侧集中管理身份、路由、分流与未命中时的出口，待应用和身份冲突等状态保持可见。设备和规则出口在卡片内选择候选，新增候选可定位到对应编辑区；规则名称来自已应用快照，候选编辑保留组合匹配条件与其他规则。

### 升级与验证说明

- 本次为正式版，替代 v0.2.4 成为稳定版 Latest；App 的稳定版更新检查可以发现 v0.3.0。
- 从 rc.6 晋升正式版时，仅更新版本标识、发布说明与文档，不引入新的网关数据面变更。下游 IPv6 仍为实验性能力，适用拓扑与验证范围见项目文档。
- 本次完成 Web/Go/桌面端测试、安装器契约检查和双架构打包校验；晋升正式版时未重新运行 Virtual Lab 或真实网关网络验收。真实 PKG 升级后重新登录，以及 macOS 13 / Intel 实机界面仍未验收。

### 开发幕后：Team Cross

OpenSurge 的开发常要在 Mac 与多台下游设备上验证网络路径；调查和修复也可能跨越多台机器、多个 Agent Session，测试证据、关键判断和后续任务需要在这些会话之间持续传递。最初，我为自己的工作流做了一个多 Agent 协作工具。看到它在 OpenSurge 开发中显著提升了开发效率后，我意识到同样的痛点也存在于团队协作、产研协同等更广泛的场景。于是，我结合一线开发团队中积累的经验，围绕会话分享、证据讨论和任务交接重新设计这个工具，将它的能力扩展到真实的团队协作场景上；最终将它发展为独立的开源项目 [Team Cross](https://github.com/YTwsy/Team-Cross)。

https://github.com/YTwsy/Team-Cross

Team Cross 让团队成员和各自的 Agent 预览并分享选定的 Codex、Claude Code 会话材料，在原文旁批注和引用；需要共同执行时，再明确开放访问，并交接或收回共享会话的输入权。在最近几个版本的 OpenSurge 开发中，它帮助我把多设备测试的上下文带入需求推进、问题排查和 PR 代码审查，减少反复解释背景与重新定位结论的工作，让这些任务更快接续和推进。

### 下载安装包

向下滚动到本页底部的 **Assets（资源）**，展开后按你的 Mac 芯片下载对应的 **`.pkg` 安装包**：

| Mac 类型 | Assets 中的安装包 | 最低系统 |
| --- | --- | --- |
| Apple Silicon（M1 及更新芯片） | **Apple Silicon 安装包**（文件名以 `arm64-unsigned.pkg` 结尾） | macOS 13+ |
| Intel Mac | **Intel 安装包**（文件名以 `x86_64-unsigned.pkg` 结尾） | macOS 13+ |

不确定芯片类型时，点击左上角 ** → 关于本机**，查看“芯片”或“处理器”。`Source code (zip)` 和 `Source code (tar.gz)` 是源码，安装 App 无需下载。

> 安装包未进行 Developer ID 签名或 notarization。本 Release 同时提供 `SHA256SUMS` 和 GitHub build provenance，供下载后核验。

### 安装

1. 下载与你的 Mac 芯片匹配的安装包。
2. 双击安装包。如果 macOS 阻止打开，请进入**系统设置 → 隐私与安全性**，选择**仍要打开**并完成身份验证，然后重新打开安装包。
3. 安装完成后，从 `/Applications` 打开 **OpenSurge**。

安装完成后，网关默认保持停止；只有在 OpenSurge 控制面中明确操作后才会启动。

<details>
<summary>可选：校验下载文件</summary>

下载 `SHA256SUMS`，运行 `shasum -a 256 安装包名称`，并与文件中的对应记录比较。

也可以使用 GitHub CLI 核对安装包的构建来源：

```sh
gh attestation verify OpenSurge-for-Mac-*-arm64-unsigned.pkg \
  -R YTwsy/OpenSurge-for-Mac
```

Intel 安装包请将命令中的 `arm64` 替换为 `x86_64`。

</details>

### 许可证

OpenSurge 自有代码采用 `GPL-3.0-only`。第三方许可证、声明与准确的对应源码链接会安装到：

`/Library/Application Support/OpenSurge/share/licenses/`

- OpenSurge-patched Mihomo `1.19.30-opensurge.1` 的上游基线源码：<https://github.com/MetaCubeX/mihomo/tree/ac017cdd246ce8bd547653d927e7bf77d7ee73d5>
- dnsmasq 2.93 源码：<https://thekelleys.org.uk/dnsmasq/dnsmasq-2.93.tar.gz>

---

## English

### v0.3.0 highlights since v0.2.4

v0.3.0 is the stable release of Verdilion, introducing a standalone macOS desktop app, a redesigned menu-bar status panel and a device workbench. It brings together the desktop, device-management and status improvements from all six release candidates.

- **Consistent, accurate gateway status:** The menu bar, sidebar and overview distinguish startup, reload, shutdown, recovery and rollback progress. Pending configuration and diagnostic reminders appear separately, while actual service failures show a specific cause. Operation boundaries are checked again, and late responses or old operation records cannot replace current state.
- **A complete desktop entry point:** Overview, network settings, devices, connections, policies and diagnostics now open in a dedicated desktop window with native menus, shortcuts, file selection and saving. Hiding and reopening the window preserves the current page and drafts. The menu bar retains an Open in Browser action.
- **A more useful menu-bar panel:** Upload/download rates, a 60-second traffic trend, local egress and active devices are available at a glance. Network details, local egress and individual devices expand inside the panel, with transitions and content-driven window sizing. Stopping the gateway replaces empty charts with a compact status; menu-bar traffic sampling continues independently while the desktop window is hidden.
- **Unified settings and appearance:** OpenSurge → Settings…, `⌘,` and the sidebar lead to the same separate settings window for language, appearance, login display, temporary lid-closed operation and updates, with quit and uninstall actions. New GitHub repository and Documentation links open in the system browser from the top of Settings. The flat sidebar status area expands quick settings through a gear icon. Title bars, selects, scrollbars and text selection follow the OpenSurge visual style.
- **Smoother window behavior:** Dock visibility follows the desktop and settings windows. A light gradient and the OpenSurge icon cover startup loading. Clicking the app icon while the app runs in the background with its windows closed reopens the main window, fixing an unwanted Settings window and a briefly flashing menu-bar panel. Quit/uninstall confirmations come to the foreground more reliably, with service reconnection and UI state preservation.
- **Recoverable login registration:** Fixes Show at login becoming disabled after an app update and incorrectly asking users to open the installed app. A missing login item for the current version can be registered again from Settings. Approval requirements and registration errors reflect the actual macOS result instead of assuming the switch change succeeded.
- **Node test results as they finish:** Each completed node test updates its result without waiting for the batch. Node tests preserve the actual gateway state instead of incorrectly showing Updating gateway. This release also improves connectivity text contrast, policy control alignment, device navigation from the menu bar and horizontal scrolling within diagnostic logs.
- **Device workbench and outlet controls:** Search and select devices on the left, then manage identity, routing, rules and unmatched outlets on the right while keeping pending changes and identity conflicts visible. Device and rule candidates expand inside the card, and Add outlet candidates opens the corresponding editor. Applied snapshots supply rule names; candidate edits preserve compound matches and other rules.

### Upgrade and validation notes

- This stable release replaces v0.2.4 as Latest. The app's stable-release update check can discover v0.3.0.
- Promotion from rc.6 updates release identity, notes and documentation without introducing new gateway data-plane changes. Downstream IPv6 remains experimental; supported topologies and validation limits are documented in the project.
- Validation covers Web/Go/desktop tests, installer contracts and both architecture packages. Virtual Lab and real gateway/network acceptance were not rerun for stable promotion. Login continuity after a real PKG upgrade and macOS 13 / Intel hardware UI behavior remain unverified.

### Behind the development: Team Cross

OpenSurge development often requires validating the same network path on a Mac and multiple downstream devices. Investigations and fixes can span several machines and Agent Sessions, so test evidence, key findings, and next steps need to travel between them. I initially built a multi-agent collaboration tool for my own workflow. Seeing how much it accelerated OpenSurge development made me realize that teams face the same challenges when working together across product and engineering. Drawing on my experience in development teams, I redesigned and rebuilt the tool around sharing sessions, discussing evidence, and handing off tasks. What began as a personal tool became the independent open-source project [Team Cross](https://github.com/YTwsy/Team-Cross).

Team Cross lets teammates and their agents preview and share selected Codex and Claude Code session material, annotate and cite passages alongside the original, and explicitly grant access when they need to work together. They can hand over or reclaim input control of a shared session. In recent OpenSurge versions, it has helped me carry multi-device test context into feature development, troubleshooting, and PR code review. That reduces repeated explanations and the need to rediscover conclusions, helping these tasks continue and move forward faster.

### Download the installer

Scroll to **Assets** at the bottom of this release page, expand the list, and download the **`.pkg` installer** for your Mac:

| Mac | Installer in Assets | Minimum system |
| --- | --- | --- |
| Apple Silicon (M1 or newer) | **Apple Silicon installer** (filename ends in `arm64-unsigned.pkg`) | macOS 13+ |
| Intel Mac | **Intel installer** (filename ends in `x86_64-unsigned.pkg`) | macOS 13+ |

Unsure which chip you have? Open **Apple menu → About This Mac** and check Chip or Processor. `Source code (zip)` and `Source code (tar.gz)` contain source files; you do not need them to install the app.

> The installers are not Developer ID signed or notarized. This release provides `SHA256SUMS` and GitHub build provenance for post-download verification.

### Install

1. Download the package matching your Mac.
2. Double-click the package. If macOS blocks it, open **System Settings → Privacy & Security**, choose **Open Anyway**, authenticate, and reopen the package.
3. After installation, open **OpenSurge** from `/Applications`.

The gateway remains stopped after installation and starts only when explicitly requested from the OpenSurge control plane.

<details>
<summary>Optional: verify the download</summary>

Download `SHA256SUMS`, run `shasum -a 256 PACKAGE_NAME`, and compare the result with the corresponding entry.

You can also verify the package's GitHub build provenance:

```sh
gh attestation verify OpenSurge-for-Mac-*-arm64-unsigned.pkg \
  -R YTwsy/OpenSurge-for-Mac
```

For the Intel package, replace `arm64` with `x86_64`.

</details>

### License

OpenSurge original code is licensed under `GPL-3.0-only`. Third-party license texts, notices, and exact corresponding-source links are installed under:

`/Library/Application Support/OpenSurge/share/licenses/`

- Upstream baseline source for OpenSurge-patched Mihomo `1.19.30-opensurge.1`: <https://github.com/MetaCubeX/mihomo/tree/ac017cdd246ce8bd547653d927e7bf77d7ee73d5>
- dnsmasq 2.93 source: <https://thekelleys.org.uk/dnsmasq/dnsmasq-2.93.tar.gz>
