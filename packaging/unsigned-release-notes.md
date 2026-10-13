[简体中文](#v031-主要变化相对-v030) · [English](#english)

> **v0.3 series codename: Verdilion**

### v0.3.1 主要变化（相对 v0.3.0）

v0.3.1 是 Verdilion 系列的修复与体验改进版本，重点改善总览图表、拓扑切换、DHCP 运行控制与菜单栏操作。

- **总览绘制开销降低，切页保留流量历史**：移除图表逐帧插值和持续 LIVE 动画，数字与曲线随采样更新。窗口可见时，切换页面继续保留最近 60 秒、最多 30 个采样；隐藏窗口暂停采样，返回后淘汰过期数据。纵轴显示清晰刻度，上传与下载趋势共用量程，相同历史不会因切页重新初始化。受控 UI 对比的方法和范围见 [#84](https://github.com/YTwsy/OpenSurge-for-Mac/issues/84)，修复见 [#85](https://github.com/YTwsy/OpenSurge-for-Mac/pull/85)。
- **直连主路由设备可保留并切换旁路由模式**：已有设备选择「直连主路由」时，从 DHCP 接管切换到手工旁路由模式，不再被已停用的 DHCP 绕行参数阻止保存或启动。设备资料、规则与出口选择继续保留；返回 DHCP 接管时重新执行完整校验。旁路由模式下，设备的实际网关和 DNS 仍需手工配置，本次不会自动改变设备的实际出口。见 [#87](https://github.com/YTwsy/OpenSurge-for-Mac/pull/87)。
- **DHCP 接管后可以直接停止网关**：「网关运行控制」集中显示运行状态与停止按钮，无需先完成或跳过客户端检查。停止操作继续使用已有的断网确认与恢复流程；可选的设备检查移入默认折叠的「下一步：检查设备接入」，接管和恢复步骤在运行期间收起，必要的恢复动作保持可见。见 [#88](https://github.com/YTwsy/OpenSurge-for-Mac/pull/88)。
- **菜单栏更多操作完整显示**：修复总览窗口较短时，菜单被裁剪、底部退出入口需要滚动查找的问题。更多操作改为独立的原生子窗口，按菜单内容与屏幕可用高度调整，打开菜单保持总览窗口尺寸；屏幕空间不足时滚动上方内容，两个退出入口仍保持可见。支持 Escape 返回触发按钮、点击外部关闭，并同步主题与语言。见 [#91](https://github.com/YTwsy/OpenSurge-for-Mac/pull/91)。
- **文档入口更清晰**：用户操作指南统一整理到 `docs/user/`，工程规范、验证方法与历史证据各有对应入口，并补充网关 DNS 与 fake-IP 的职责说明。

### 升级与验证说明

- 本次为正式版，替代 v0.3.0 成为稳定版 Latest；App 的稳定版更新检查可以发现 v0.3.1。
- 下游 IPv6 仍为实验性能力，适用拓扑与验证范围见项目文档。旁路由模式的配置保留不代表已自动修改下游设备的网络设置。
- 发布检查覆盖 Go/Web/桌面端测试、嵌入资源构建、安装器契约，以及 Apple Silicon / Intel 双架构安装包校验。拓扑切换修复的回归覆盖配置校验、生成结果和界面行为；本次未完成 Virtual Lab 或真实 DHCP/TUN 网络验收。真实 PKG 升级后重新登录，以及 macOS 13 / Intel 实机界面仍未验收。
- 完整变更：[v0.3.0...v0.3.1](https://github.com/YTwsy/OpenSurge-for-Mac/compare/v0.3.0...v0.3.1)。

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

### v0.3.1 highlights since v0.3.0

v0.3.1 is a maintenance and usability release in the Verdilion series, focused on overview charts, topology switching, DHCP gateway controls and menu-bar actions.

- **Less overview rendering overhead, with traffic history preserved across pages:** Removes per-frame chart interpolation and the continuous LIVE animation. Numbers and curves update with each sample. While the window is visible, navigation preserves up to 30 samples from the last 60 seconds; hiding pauses sampling, and returning discards expired data. Labeled axes and a shared upload/download trend scale keep the same history consistent across page changes. The controlled UI comparison and its limits are documented in [#84](https://github.com/YTwsy/OpenSurge-for-Mac/issues/84); the fix is in [#85](https://github.com/YTwsy/OpenSurge-for-Mac/pull/85).
- **Keep main-router devices when switching to manual bypass-router mode:** Devices configured to use the main router no longer prevent saving or starting manual bypass-router mode because of inactive DHCP bypass settings. Device identities, rules and outlet selections are retained, and full validation resumes when DHCP takeover is enabled again. Clients still need manual gateway and DNS configuration in bypass-router mode; this change does not automatically switch their actual egress. See [#87](https://github.com/YTwsy/OpenSurge-for-Mac/pull/87).
- **Stop a DHCP gateway directly:** Gateway running controls now show status and a Stop action without requiring client checks to be completed or skipped first. Existing network-disruption confirmation and recovery flows remain in use. Optional client checks move into a collapsed next-step section; takeover and recovery instructions collapse while running, while required recovery actions stay visible. See [#88](https://github.com/YTwsy/OpenSurge-for-Mac/pull/88).
- **Menu-bar actions remain fully accessible:** Fixes the More actions menu being clipped by a short overview window. Actions open in a separate native child window sized to the menu and available screen height, keeping the overview size unchanged. On short screens, the upper content scrolls while both quit actions remain visible. Escape returns focus to the trigger, clicking outside closes the menu, and theme and language stay synchronized. See [#91](https://github.com/YTwsy/OpenSurge-for-Mac/pull/91).
- **Clearer documentation entry points:** User guides are consolidated under `docs/user/`, with separate entry points for engineering contracts, validation methods and historical evidence, plus clearer gateway DNS and fake-IP responsibilities.

### Upgrade and validation notes

- This stable release replaces v0.3.0 as Latest. The app's stable-release update check can discover v0.3.1.
- Downstream IPv6 remains experimental; topology requirements and validation limits are documented in the project. Retaining configuration in bypass-router mode does not automatically change client network settings.
- Release checks cover Go/Web/desktop tests, embedded-resource builds, installer contracts and Apple Silicon / Intel package verification. Topology-switching regressions cover configuration validation, generated output and UI behavior; Virtual Lab and real DHCP/TUN network acceptance were not completed for this release. Login continuity after a real PKG upgrade and macOS 13 / Intel hardware UI behavior remain unverified.
- Full changelog: [v0.3.0...v0.3.1](https://github.com/YTwsy/OpenSurge-for-Mac/compare/v0.3.0...v0.3.1).

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
