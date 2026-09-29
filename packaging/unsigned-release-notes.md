[简体中文](#简体中文) · [English](#english)

> **v0.3 系列代号：Verdilion · 第二个候选版本**<br>
> **v0.3 series codename: Verdilion · Second release candidate**

## 简体中文

### v0.3.0-rc.2 主要变化（相对 v0.2.4）

Verdilion 带来独立的 macOS 桌面 App，以及重新整理的菜单栏状态面板。本次是基于 `Next` 的第二个 v0.3 候选版本，面向愿意提前体验并反馈问题的用户；以下主要变化以 v0.2.4 为基准，稳定版仍为 v0.2.4。

- **完整的桌面操作入口**：OpenSurge 现在直接在独立桌面窗口中展示总览、网络设置、设备、连接、策略和诊断。窗口支持原生菜单、快捷键、文件选择与保存；隐藏后重新打开会保留当前页面和草稿。菜单栏也保留「在浏览器中打开」入口。
- **更实用的菜单栏状态面板**：集中展示上传下载速率、近 60 秒趋势、本机出口和活跃设备。网络状态、本机出口与具体设备可在面板内展开；窗口随内容调整大小，并提供展开过渡。网关停止时收起流量曲线，显示紧凑提示；桌面窗口隐藏后，菜单栏仍独立采样流量。
- **统一的设置入口与外观**：通过「OpenSurge → 设置…」、`⌘,` 或侧栏快捷设置进入独立设置窗口，管理语言、外观、登录时显示、临时合盖保持运行与更新，并提供退出和卸载入口。设置顶部新增「GitHub 仓库」和「文档」链接，点击后在系统浏览器中打开。侧栏底部保留扁平状态样式，齿轮可展开快捷设置；标题栏、列表、滚动条与文字选择统一使用 OpenSurge 的界面风格。
- **更连贯的窗口体验**：Dock 图标跟随桌面和设置窗口的显示状态；启动加载阶段展示浅色渐变与 OpenSurge 图标。App 在后台运行且窗口已关闭时，再次点击图标会重新显示主窗口，修复设置窗口被一同打开、菜单栏面板闪现的问题。同步改善退出和卸载确认窗口的前台显示，以及与后台服务断开后的重连和状态保留。
- **节点检测逐项显示结果**：策略页每完成一个节点的检测就更新该节点结果，无需等待整批完成。同步改善连通性页面文字对比度、策略页控件对齐、菜单栏设备跳转，以及诊断日志的局部横向滚动。

### 候选版本说明

- 本次以 GitHub Pre-release 发布，不替代 v0.2.4 的稳定版 Latest，也不合入 `master`。
- 更新检查继续只发现稳定版；安装本候选版本后，后续 RC 需要从 GitHub Releases 手动获取。
- 网关规则、配置与网络数据面沿用 v0.2.4 的实现边界。下游 IPv6 仍为实验性能力，适用拓扑与验证范围见项目文档。
- 本次完成 Web/Go/桌面端测试、安装器契约检查、隔离界面检查和双架构打包校验；未为本 RC 重新运行 Virtual Lab 或真实网关网络验收。隔离界面检查不等于真实安装升级或 Intel 实机界面验收。

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

### v0.3.0-rc.2 highlights since v0.2.4

Verdilion introduces a standalone macOS desktop app and a redesigned menu-bar status panel. This is the second v0.3 release candidate based on `Next`, intended for users who want to try the new experience and report issues. The highlights below compare against v0.2.4, which remains the stable release.

- **A complete desktop entry point:** Overview, network settings, devices, connections, policies and diagnostics now open in a dedicated desktop window with native menus, shortcuts, file selection and saving. Hiding and reopening the window preserves the current page and drafts. The menu bar retains an Open in Browser action.
- **A more useful menu-bar panel:** Upload/download rates, a 60-second traffic trend, local egress and active devices are available at a glance. Network details, local egress and individual devices expand inside the panel, with transitions and content-driven window sizing. Stopping the gateway replaces empty charts with a compact status; menu-bar traffic sampling continues independently while the desktop window is hidden.
- **Unified settings and appearance:** OpenSurge → Settings…, `⌘,` and the sidebar lead to the same separate settings window for language, appearance, login display, temporary lid-closed operation and updates, with quit and uninstall actions. New GitHub repository and Documentation links open in the system browser from the top of Settings. The flat sidebar status area expands quick settings through a gear icon. Title bars, selects, scrollbars and text selection follow the OpenSurge visual style.
- **Smoother window behavior:** Dock visibility follows the desktop and settings windows. A light gradient and the OpenSurge icon cover startup loading. Clicking the app icon while the app runs in the background with its windows closed reopens the main window, fixing an unwanted Settings window and a briefly flashing menu-bar panel. Quit/uninstall confirmations come to the foreground more reliably, with service reconnection and UI state preservation.
- **Node test results as they finish:** Each completed node test updates its result without waiting for the batch. This release also improves connectivity text contrast, policy control alignment, device navigation from the menu bar and horizontal scrolling within diagnostic logs.

### Release-candidate notes

- Published as a GitHub Pre-release, without replacing v0.2.4 as the stable Latest release or merging into `master`.
- Update checks still discover stable releases only. Subsequent RCs must be downloaded manually from GitHub Releases.
- Gateway rules, configuration and the network data plane retain their v0.2.4 implementation boundaries. Downstream IPv6 remains experimental; supported topologies and validation limits are documented in the project.
- Validation covers Web/Go/desktop tests, installer contracts, isolated UI checks and both architecture packages. Virtual Lab and real gateway/network acceptance were not rerun for this RC. Isolated UI checks do not establish a real installation/upgrade or Intel hardware UI acceptance.

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
