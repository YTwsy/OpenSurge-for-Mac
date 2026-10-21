[简体中文](#v032-主要变化相对-v031) · [English](#english)

> **v0.3 series codename: Verdilion**

### v0.3.2 主要变化（相对 v0.3.1）

v0.3.2 新增带密码的局域网管理和 Mac 本机 IPv6 独立开关，改善手机操作与连接检查体验，并提供文件式自定义 DNS；同时修复订阅 DNS 兼容、安装版预览权限及网关重载恢复问题。

- **从同一局域网的手机或其他电脑管理 OpenSurge**：在“App 设置与更新 → 局域网管理”中开启访问、设置管理密码，再通过地址或二维码打开浏览器控制台。支持查看状态、管理来源、设备与策略，以及执行网络控制。功能默认关闭，二维码只包含访问地址；修改密码或关闭入口会使现有远程会话失效。当前使用 HTTP 和私有 IPv4 地址，适用于受信任的局域网。见 [#99](https://github.com/YTwsy/OpenSurge-for-Mac/pull/99)。
- **独立控制 Mac 本机 IPv6 接管**：网络设置新增“Mac 本机 IPv6 接管”，默认开启，旧配置保持原行为。关闭后保存并完整重启网关，可撤销本机 TUN 的 IPv6 地址与捕获路由；DNS AAAA、下游 IPv6 接管和 macOS 原生 IPv6 仍各自独立。见 [#98](https://github.com/YTwsy/OpenSurge-for-Mac/pull/98)。
- **改善手机界面与连接检查体验**：为各主要控制页面调整窄屏布局、导航抽屉和菜单交互。连接详情改为独立浮层：桌面可以拖动，手机在底部展示，查看详情时仍可筛选和操作列表；连接离开当前快照后保留最后一次观察到的详情。设备与本机设置补充加载、读取失败和重试反馈。见 [#99](https://github.com/YTwsy/OpenSurge-for-Mac/pull/99)。
- **新增文件式自定义 DNS**：在“代理与规则源”中导入 hosts 文件，或仅包含 `hosts` / `dns` 的 Mihomo YAML，为 NAS 等服务设置固定地址、为公司域名指定内网 DNS，或配置返回真实地址的例外。自定义 DNS 独立于全局附加配置，支持草稿、预览、导出和显式应用，并统一用于 Mac TUN 与使用 OpenSurge DNS 的下游设备。保存草稿不会立即改变网络；应用前会检查接入条件、DHCP 名称冲突和已知解析循环。见 [#97](https://github.com/YTwsy/OpenSurge-for-Mac/pull/97)。
- **修复订阅 DNS 兼容与自定义 DNS 预览**：修复 [#93](https://github.com/YTwsy/OpenSurge-for-Mac/issues/93)：保留订阅中的 `hosts` 映射及配套 DNS 策略，将能够明确识别的订阅自身 DNS 节点解析引用绑定到当前核心，避免监听端口调整后仍访问旧端口。网关停止时，准备态按需提供私有 DNS 监听，支持节点测试和 Provider 解析。此兼容修复默认生效，无需开启自定义 DNS。另修复安装版自定义 DNS 预览读取受保护配置时的权限错误。相关改动见 [#95](https://github.com/YTwsy/OpenSurge-for-Mac/pull/95)、[#97](https://github.com/YTwsy/OpenSurge-for-Mac/pull/97)。
- **修复重载失败后的恢复状态，完善启动就绪等待**：改善核心启动较慢时的有界等待与错误诊断。应用新配置失败后，根据旧网关的实际恢复结果更新 DHCP 接管状态，修复旧网关已恢复运行、界面却退回待启动阶段的问题；恢复未完成时保留真实错误和后续恢复入口。应用请求取消后，也会继续尝试恢复已经停止的旧网关。见 [#100](https://github.com/YTwsy/OpenSurge-for-Mac/pull/100)。
- **修复 IPv6 核心冷启动并加强安装包构建校验**：修复无原生 IPv6 上游、AAAA 关闭而本机 IPv6 接管开启时的补丁核心冷启动行为。安装包按固定源码和当前补丁构建核心，仅在构建输入与二进制摘要匹配时复用缓存，并检查实际入包的核心，避免旧缓存遗漏补丁变化。见 [#98](https://github.com/YTwsy/OpenSurge-for-Mac/pull/98)。

### 升级与验证说明

- 自定义 DNS 默认关闭，保存、启用或禁用草稿后，都需要显式应用才会改变运行配置。
- Mac 本机 IPv6 接管默认开启，修改后需要完整重启网关。
- 局域网管理默认关闭；停止网关或仅退出桌面 App，不会关闭已经启用的管理入口。请在设置中关闭并保存。
- 本次为正式版，替代 v0.3.1 成为稳定版 Latest；App 的稳定版更新检查可以发现 v0.3.2。
- 发布检查覆盖 Go/Web/桌面端测试、嵌入资源构建、安装器契约，以及 Apple Silicon / Intel 双架构安装包校验。已有 DNS、Mac IPv6 与安装态验收沿用各项记录的版本和范围；本轮未新增 Virtual Lab 或真实 DHCP/TUN 数据面复验。DNS 自身引用的完整正式端口矩阵、局域网管理物理设备验收等未覆盖项见[验证范围与证据入口](https://github.com/YTwsy/OpenSurge-for-Mac/blob/v0.3.2/docs/agent-wiki/sources/validation/evidence-map.md)。
- 完整变更：[v0.3.1...v0.3.2](https://github.com/YTwsy/OpenSurge-for-Mac/compare/v0.3.1...v0.3.2)。

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

### v0.3.2 highlights since v0.3.1

v0.3.2 adds password-protected LAN management and independent Mac IPv6 control, improves mobile controls and connection inspection, and introduces file-based Custom DNS. It also fixes subscription DNS compatibility, installed-build preview permissions and gateway reload recovery.

- **Manage OpenSurge from another device on your LAN:** Enable LAN management in App Settings & Updates, set a management password, and open the browser control panel using its URL or QR code. Manage status, sources, devices, policies and network controls from a phone or another computer. The feature is off by default, and the QR contains only the URL. Changing the password or disabling access revokes existing remote sessions. This version uses HTTP and private IPv4 addresses and is intended for trusted LANs. See [#99](https://github.com/YTwsy/OpenSurge-for-Mac/pull/99).
- **Control Mac IPv6 takeover independently:** Adds a Mac IPv6 takeover setting, enabled by default to preserve existing behavior. Save and fully restart the gateway after changing it. Disabling removes the Mac TUN IPv6 addresses and capture routes; DNS AAAA, downstream IPv6 takeover and native macOS IPv6 remain independently controlled. See [#98](https://github.com/YTwsy/OpenSurge-for-Mac/pull/98).
- **Improve mobile controls and connection inspection:** Refines narrow-screen layouts, navigation drawers and menus across the main control pages. Connection details open in a draggable desktop panel or a bottom panel on mobile, allowing continued filtering and list interaction. The selected connection retains its last observed details after leaving the current snapshot. Device and Mac settings also gain clearer loading, failure and retry feedback. See [#99](https://github.com/YTwsy/OpenSurge-for-Mac/pull/99).
- **Add file-based Custom DNS:** Import a hosts file or Mihomo YAML containing only `hosts` / `dns` from Sources to configure fixed service addresses, corporate DNS routing and real-IP exceptions. Custom DNS is independent of the Global Profile Overlay, with drafts, previews, export and explicit application for Mac TUN and downstream devices using OpenSurge DNS. Saving does not change the running gateway. Application checks prerequisites, DHCP namespace conflicts and known resolution loops. See [#97](https://github.com/YTwsy/OpenSurge-for-Mac/pull/97).
- **Fix subscription DNS compatibility and Custom DNS previews:** Fixes [#93](https://github.com/YTwsy/OpenSurge-for-Mac/issues/93): preserves subscription `hosts` mappings and associated DNS policies, and binds confirmed references to the subscription’s own DNS for node resolution to the current core, avoiding stale resolver ports after listener changes. While the gateway is stopped, the prepared core provides a private DNS listener when needed for node tests and Providers. This compatibility fix works without enabling Custom DNS. Also fixes permission errors when previewing protected configuration in installed builds. Related changes: [#95](https://github.com/YTwsy/OpenSurge-for-Mac/pull/95), [#97](https://github.com/YTwsy/OpenSurge-for-Mac/pull/97).
- **Fix recovery state after failed reloads and improve startup readiness:** Improves bounded waits and diagnostics for slower core startup. When applying a new configuration fails, DHCP takeover state now follows the actual recovery result, fixing cases where the old gateway had recovered but the interface returned to a pending-start stage. Incomplete recovery retains the error and recovery actions. Cancelling an apply request still attempts to restore an old gateway that has already been stopped. See [#100](https://github.com/YTwsy/OpenSurge-for-Mac/pull/100).
- **Fix IPv6 core cold startup and strengthen installer build verification:** Fixes patched-core cold startup without native IPv6 when AAAA is disabled and Mac IPv6 takeover is enabled. Packaging builds the core from pinned source and current patches, reuses cached binaries only when build inputs and binary digests match, and checks the exact core included in the installer. See [#98](https://github.com/YTwsy/OpenSurge-for-Mac/pull/98).

### Upgrade and validation notes

- Custom DNS is off by default. Saved changes, including enabling or disabling it, require explicit application.
- Mac IPv6 takeover remains enabled by default. Changing it requires a full gateway restart.
- LAN management is off by default. Stopping the gateway or quitting only the desktop app leaves an enabled management endpoint running; disable and save it in Settings to close access.
- This stable release replaces v0.3.1 as Latest. The app's stable-release update check can discover v0.3.2.
- Release checks cover Go/Web/desktop tests, embedded-resource builds, installer contracts and Apple Silicon / Intel package verification. Existing DNS, Mac IPv6 and installed-build acceptance retains each record’s tested version and scope. This release adds no Virtual Lab or live DHCP/TUN data-plane revalidation. The full formal-port self-DNS matrix, physical-device LAN management acceptance and other uncovered cases are documented in the [validation evidence map](https://github.com/YTwsy/OpenSurge-for-Mac/blob/v0.3.2/docs/agent-wiki/sources/validation/evidence-map.md).
- Full changelog: [v0.3.1...v0.3.2](https://github.com/YTwsy/OpenSurge-for-Mac/compare/v0.3.1...v0.3.2).

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
