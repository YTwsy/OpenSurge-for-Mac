[简体中文](#v030-rc5-主要变化相对-v030-rc4) · [English](#english)

> **v0.3 series codename: Verdilion · Fifth release candidate**

### v0.3.0-rc.5 主要变化（相对 v0.3.0-rc.4）

本次是基于 `Next` 的第五个 v0.3 候选版本，重点修复菜单栏、桌面左下角与总览卡片的网关状态显示，面向愿意提前体验并反馈问题的用户。稳定版仍为 v0.2.4。

- **准确区分操作中与运行异常**：启动、应用配置、停止、代理引擎恢复与回滚分别显示对应状态和当前阶段。启动或重载中途组件尚未就绪时，不再误报整个网关运行异常；菜单栏、侧栏与总览卡片使用一致的状态与颜色。
- **配置与诊断提醒更清楚**：新增设备出口等配置修改后，正常运行的网关继续显示“正在运行”，另行提示有配置待应用、当前仍使用原配置。上次诊断有检查项时单独提示，不再直接把网关标为异常。
- **故障原因与暂不可用分开显示**：代理引擎退出、TUN 故障、PF 未加载或 IPv4 转发关闭时显示具体原因；无法确认运行状态或操作结果时显示暂不可用提示。操作仍在执行或状态未确认时，完整退出和卸载会继续等待确认，避免把中途停止当作操作结束。
- **减少旧状态闪现**：操作开始和完成时及时刷新；操作期间加快菜单栏状态采样。较早查询的迟到结果不会覆盖最新状态，跨越操作边界的采样会重新确认，遗留操作记录也不会让界面一直停在“启动中”。

### v0.3 系列主要变化（相对 v0.2.4）

v0.3 带来独立的 macOS 桌面 App，以及重新整理的菜单栏状态面板。以下变化包含前四个候选版本的桌面与设备管理改进。

- **完整的桌面操作入口**：OpenSurge 现在直接在独立桌面窗口中展示总览、网络设置、设备、连接、策略和诊断。窗口支持原生菜单、快捷键、文件选择与保存；隐藏后重新打开会保留当前页面和草稿。菜单栏也保留「在浏览器中打开」入口。
- **更实用的菜单栏状态面板**：集中展示上传下载速率、近 60 秒趋势、本机出口和活跃设备。网络状态、本机出口与具体设备可在面板内展开；窗口随内容调整大小，并提供展开过渡。网关停止时收起流量曲线，显示紧凑提示；桌面窗口隐藏后，菜单栏仍独立采样流量。
- **统一的设置入口与外观**：通过「OpenSurge → 设置…」、`⌘,` 或侧栏快捷设置进入独立设置窗口，管理语言、外观、登录时显示、临时合盖保持运行与更新，并提供退出和卸载入口。设置顶部新增「GitHub 仓库」和「文档」链接，点击后在系统浏览器中打开。侧栏底部保留扁平状态样式，齿轮可展开快捷设置；标题栏、列表、滚动条与文字选择统一使用 OpenSurge 的界面风格。
- **更连贯的窗口体验**：Dock 图标跟随桌面和设置窗口的显示状态；启动加载阶段展示浅色渐变与 OpenSurge 图标。App 在后台运行且窗口已关闭时，再次点击图标会重新显示主窗口，修复设置窗口被一同打开、菜单栏面板闪现的问题。同步改善退出和卸载确认窗口的前台显示，以及与后台服务断开后的重连和状态保留。
- **登录项可以重新注册**：修复更新 App 后「登录时显示」开关被禁用、误提示需要打开已安装 App 的问题。当前版本的登录项缺失时，可在设置中重新开启；等待 macOS 批准或注册失败会显示真实状态和错误，不会把点击开关当作注册成功。
- **节点检测逐项显示结果**：策略页每完成一个节点的检测就更新该节点结果，无需等待整批完成。同步改善连通性页面文字对比度、策略页控件对齐、菜单栏设备跳转，以及诊断日志的局部横向滚动。
- **设备工作台与出口操作**：左侧搜索和选择设备，右侧集中管理身份、路由、分流与未命中时的出口，待应用和身份冲突等状态保持可见。设备和规则出口在卡片内选择候选，新增候选可定位到对应编辑区；规则名称来自已应用快照，候选编辑保留组合匹配条件与其他规则。

### 候选版本说明

- 本次以 GitHub Pre-release 发布，不替代 v0.2.4 的稳定版 Latest，也不合入 `master`。
- 更新检查继续只发现稳定版；安装本候选版本后，后续 RC 需要从 GitHub Releases 手动获取。
- 网关规则、配置与网络数据面沿用 v0.2.4 的实现边界。下游 IPv6 仍为实验性能力，适用拓扑与验证范围见项目文档。
- 本次完成 Web/Go/桌面端测试、安装器契约检查和双架构打包校验；未为本 RC 重新运行 Virtual Lab 或真实网关网络验收。真实 PKG 升级后重新登录，以及 Intel 实机界面仍未验收。

### 选择安装包

| Mac 类型 | 安装包 | 最低系统 |
| --- | --- | --- |
| Apple Silicon（M1 及更新芯片） | `arm64-unsigned.pkg` | macOS 13+ |
| Intel Mac | `x86_64-unsigned.pkg` | macOS 13+ |

> 安装包未进行 Developer ID 签名或 notarization。此 Pre-release 同时提供 `SHA256SUMS` 和 GitHub build provenance，供下载后核验。

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

### v0.3.0-rc.5 highlights since v0.3.0-rc.4

This is the fifth v0.3 release candidate based on `Next`, focused on accurate gateway status in the menu bar, desktop sidebar and overview card. It is intended for users who want to try the new experience and report issues. v0.2.4 remains the stable release.

- **Distinguish progress from runtime failures:** Starting, applying configuration, stopping, proxy-engine recovery and rollback show their own state and current phase. Components that are not ready during startup or reload no longer turn the entire gateway into a runtime warning. The menu bar, sidebar and overview card use consistent states and colors.
- **Clearer configuration and diagnostic reminders:** Adding device outlet candidates or editing configuration keeps a healthy gateway marked Running, with a separate reminder that changes are pending and the previous configuration is still in use. Findings from the last diagnostic run are shown separately rather than changing the gateway's runtime state.
- **Specific failures and unavailable status:** An exited proxy engine, failed TUN, missing PF rules or disabled IPv4 forwarding shows the corresponding reason. Unconfirmed runtime status or operation results show an unavailable-state message. Full quit and uninstall continue to wait while an operation is active or its status is unconfirmed, avoiding an exit during an intermediate stop.
- **Fewer stale status flashes:** Operation start and completion refresh the summaries, with faster menu-bar sampling during operations. Late responses cannot replace newer state, samples crossing operation boundaries are checked again, and old operation records cannot leave the UI stuck at Starting.

### v0.3 series highlights since v0.2.4

Verdilion introduces a standalone macOS desktop app and a redesigned menu-bar status panel. The following highlights include desktop and device-management improvements from the first four release candidates.

- **A complete desktop entry point:** Overview, network settings, devices, connections, policies and diagnostics now open in a dedicated desktop window with native menus, shortcuts, file selection and saving. Hiding and reopening the window preserves the current page and drafts. The menu bar retains an Open in Browser action.
- **A more useful menu-bar panel:** Upload/download rates, a 60-second traffic trend, local egress and active devices are available at a glance. Network details, local egress and individual devices expand inside the panel, with transitions and content-driven window sizing. Stopping the gateway replaces empty charts with a compact status; menu-bar traffic sampling continues independently while the desktop window is hidden.
- **Unified settings and appearance:** OpenSurge → Settings…, `⌘,` and the sidebar lead to the same separate settings window for language, appearance, login display, temporary lid-closed operation and updates, with quit and uninstall actions. New GitHub repository and Documentation links open in the system browser from the top of Settings. The flat sidebar status area expands quick settings through a gear icon. Title bars, selects, scrollbars and text selection follow the OpenSurge visual style.
- **Smoother window behavior:** Dock visibility follows the desktop and settings windows. A light gradient and the OpenSurge icon cover startup loading. Clicking the app icon while the app runs in the background with its windows closed reopens the main window, fixing an unwanted Settings window and a briefly flashing menu-bar panel. Quit/uninstall confirmations come to the foreground more reliably, with service reconnection and UI state preservation.
- **Recoverable login registration:** Fixes Show at login becoming disabled after an app update and incorrectly asking users to open the installed app. A missing login item for the current version can be registered again from Settings. Approval requirements and registration errors reflect the actual macOS result instead of assuming the switch change succeeded.
- **Node test results as they finish:** Each completed node test updates its result without waiting for the batch. This release also improves connectivity text contrast, policy control alignment, device navigation from the menu bar and horizontal scrolling within diagnostic logs.
- **Device workbench and outlet controls:** Search and select devices on the left, then manage identity, routing, rules and unmatched outlets on the right while keeping pending changes and identity conflicts visible. Device and rule candidates expand inside the card, and Add outlet candidates opens the corresponding editor. Applied snapshots supply rule names; candidate edits preserve compound matches and other rules.

### Release-candidate notes

- Published as a GitHub Pre-release, without replacing v0.2.4 as the stable Latest release or merging into `master`.
- Update checks still discover stable releases only. Subsequent RCs must be downloaded manually from GitHub Releases.
- Gateway rules, configuration and the network data plane retain their v0.2.4 implementation boundaries. Downstream IPv6 remains experimental; supported topologies and validation limits are documented in the project.
- Validation covers Web/Go/desktop tests, installer contracts and both architecture packages. Virtual Lab and real gateway/network acceptance were not rerun for this RC. Login continuity after a real PKG upgrade and Intel hardware UI behavior remain unverified.

### Choose a package

| Mac | Package | Minimum system |
| --- | --- | --- |
| Apple Silicon (M1 or newer) | `arm64-unsigned.pkg` | macOS 13+ |
| Intel Mac | `x86_64-unsigned.pkg` | macOS 13+ |

> The installers are not Developer ID signed or notarized. This Pre-release provides `SHA256SUMS` and GitHub build provenance for post-download verification.

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
