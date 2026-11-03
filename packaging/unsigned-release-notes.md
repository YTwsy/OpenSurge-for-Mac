[简体中文](#v033-rc1-主要变化相对-v032) · [English](#english)

> **v0.3 series codename: Verdilion**

### v0.3.3-rc.1 主要变化（相对 v0.3.2）

v0.3.3-rc.1 是 v0.3.3 的首个候选版本，重点改善网关恢复、配置管理和操作反馈，并新增默认关闭的 Sniffer 域名嗅探开关。本次以 **Pre-release** 发布，供提前体验和反馈。

- **修复 DNS 恢复失败阻断网关清理与后续启动的问题**：系统 DNS 的原设置改为独立保存；停止、回滚或重启代理引擎时按备份恢复。原网络服务已消失时跳过恢复，不把旧设置写入同名新服务；恢复失败时保留提醒和已保存的备份供重试，并继续其他网关清理，避免单个 DNS 恢复错误卡住整个流程。见 [#107](https://github.com/YTwsy/OpenSurge-for-Mac/issues/107)、[#109](https://github.com/YTwsy/OpenSurge-for-Mac/pull/109)。
- **减少无关配置错误对恢复操作的阻断**：停止网关、DHCP 恢复和状态诊断不再一并要求下一次部署的参数及未应用策略完整有效；租约读取失败单独提示，不遮蔽已知网关状态与恢复入口。独立重启代理引擎不再依赖无关的 dnsmasq 或未应用策略文件；下游接口明确消失时，仍可清理本地资源。新配置应用时继续执行完整校验。见 [#109](https://github.com/YTwsy/OpenSurge-for-Mac/pull/109)。
- **新增 Sniffer 域名嗅探总开关，默认关闭**：在“网络设置 → 高级 Mihomo / DNS 设置”中按需启用。当应用使用外部加密 DNS 或缓存的真实 IP，导致连接缺少域名映射时，可从 HTTP Host、TLS/QUIC 握手中补充域名供规则匹配。嗅探保留原始目标 IP，不改变设备规则优先级；连接页会展示嗅探域名，方便核对命中规则与出口。见 [#110](https://github.com/YTwsy/OpenSurge-for-Mac/issues/110)、[#111](https://github.com/YTwsy/OpenSurge-for-Mac/pull/111)。
- **支持删除不再使用的代理与规则源**：来源卡片新增“删除来源”，确认后清理列表记录、托管历史快照和保存的订阅地址；即使快照已在 Finder 中删除，也能清理残留记录。原始 YAML、已导出的副本和全局附加配置保留。当前选中或仍被引用的来源需要先切换，删除本身不会重载网关。见 [#105](https://github.com/YTwsy/OpenSurge-for-Mac/issues/105)。
- **统一操作通知与配置检查反馈**：导入、刷新、保存和网络操作结果使用统一悬浮通知，减少提示出现时的页面跳动。成功提示自动收起，失败提示保留供查看、复制和手动关闭；多条配置阻塞项与提醒汇总为可展开摘要。DNS 恢复失败等子步骤提醒也会独立保留，避免被整体操作成功提示掩盖。见 [#108](https://github.com/YTwsy/OpenSurge-for-Mac/pull/108)、[#109](https://github.com/YTwsy/OpenSurge-for-Mac/pull/109)。
- **改善停止状态下的连接页与本机策略布局**：正常停止网关且没有待恢复事项时，连接页显示“尚未启动网络模式”和网络设置入口，保留已知设备信息；重新启动后刷新实时连接，避免显示上一次运行的旧数据。策略页同步调整本机出口与“仅影响本机”标记的对齐和窄屏布局。
- **滚动连接列表时，随时切换设备**：设备筛选列表会吸顶停留，设备较多时可在列表内单独滚动；手机等窄屏仍可横向滑动选择。设备页“前往网络设置”按钮也统一了外观和间距，让首次配置入口更清晰。

### 升级说明

- 本次为 **Pre-release**，不替代 v0.3.2 的稳定版 Latest。App 的更新检查仍面向正式版，请从本页手动下载此 RC。
- Sniffer 在新安装和旧配置升级后均默认关闭，导入订阅不会覆盖该开关。修改时需停止网关，保存网络配置后重新启动；它不能保证识别 ECH 隐藏的真实域名或其他没有可识别域名的流量。
- Mac 系统 DNS 按接管前备份恢复；接管期间由用户或其他软件修改的 DNS 也会被原备份替换。恢复失败的提醒和已保存的备份会保留供后续重试。
- 完整变更：[v0.3.2...v0.3.3-rc.1](https://github.com/YTwsy/OpenSurge-for-Mac/compare/v0.3.2...v0.3.3-rc.1)。

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

### v0.3.3-rc.1 highlights since v0.3.2

v0.3.3-rc.1 is the first release candidate for v0.3.3. It focuses on gateway recovery, configuration management and operation feedback, and adds an opt-in domain sniffer. It is published as a **Pre-release** for early testing and feedback.

- **Fix DNS restoration blocking gateway cleanup and subsequent startup:** Original system DNS settings are now saved independently and restored during stop, rollback or proxy-engine restart. If the original network service no longer exists, restoration skips it without changing a new service with the same name. Restoration failures retain a warning and the persisted backup for retry while other gateway cleanup continues. See [#107](https://github.com/YTwsy/OpenSurge-for-Mac/issues/107), [#109](https://github.com/YTwsy/OpenSurge-for-Mac/pull/109).
- **Keep recovery actions available despite unrelated configuration errors:** Gateway stop, DHCP recovery and status diagnostics no longer require every future deployment setting and unapplied policy to be valid. Lease-reading errors are reported separately without hiding known gateway state or recovery actions. Restarting only the proxy engine no longer depends on unrelated dnsmasq or unapplied policy files; local cleanup can proceed when a downstream interface is confirmed absent. New deployments still require full validation. See [#109](https://github.com/YTwsy/OpenSurge-for-Mac/pull/109).
- **Add a Sniffer master switch, off by default:** Enable it in Network Settings → Advanced Mihomo / DNS settings when needed. If an app uses external encrypted DNS or cached real IPs and its connection lacks a domain mapping, sniffing can supplement domain rules using HTTP Host and TLS/QUIC handshakes. It retains the original destination IP and device-rule precedence. The Connections page displays sniffed domains to help inspect the matched rule and egress. See [#110](https://github.com/YTwsy/OpenSurge-for-Mac/issues/110), [#111](https://github.com/YTwsy/OpenSurge-for-Mac/pull/111).
- **Delete unused proxy and rule sources:** Source cards now provide Delete Source. After confirmation, OpenSurge removes the library entry, managed historical snapshots and saved subscription URL, including entries whose snapshots were already removed in Finder. Original YAML files, exported copies and the Global Profile Overlay are retained. Selected or referenced sources require switching away first; deletion does not reload the gateway. See [#105](https://github.com/YTwsy/OpenSurge-for-Mac/issues/105).
- **Unify operation notifications and configuration checks:** Import, refresh, save and network operations use shared floating notifications to reduce layout shifts. Success messages dismiss automatically; errors remain available for reading, copying and manual dismissal. Multiple configuration blockers and reminders are grouped into expandable summaries. Warnings from individual steps, such as DNS restoration failures, remain visible independently of an overall successful operation. See [#108](https://github.com/YTwsy/OpenSurge-for-Mac/pull/108), [#109](https://github.com/YTwsy/OpenSurge-for-Mac/pull/109).
- **Improve stopped-state connection guidance and local policy layout:** When the gateway is normally stopped with no pending recovery, Connections shows startup guidance and a link to Network Settings while retaining known device identities. Starting again refreshes live connections without reusing stale data from the previous run. The Policies page also improves alignment and narrow-screen layout for the local egress controls and Mac-only badge.
- **Switch devices while scrolling through connections:** The device filter stays visible as you scroll, with independent scrolling for longer device lists. Phones and other narrow screens retain horizontal device selection. The Devices page also gives its Go to Network Settings button consistent styling and spacing, making initial setup easier to find.

### Upgrade notes

- This is a **Pre-release** and does not replace v0.3.2 as stable Latest. The app's update check targets stable releases; download this RC manually from this page.
- Sniffer is off by default for both new installations and upgrades from older configurations. Imported subscriptions cannot override the switch. Stop the gateway, save the changed network configuration, and start it again to apply. Sniffing cannot reliably identify the real domain hidden by ECH or traffic without a recognizable hostname.
- Mac system DNS is restored from the original pre-takeover backup, replacing DNS changes made by the user or other software during takeover. Failed restoration retains a warning and the persisted backup for retry.
- Full changelog: [v0.3.2...v0.3.3-rc.1](https://github.com/YTwsy/OpenSurge-for-Mac/compare/v0.3.2...v0.3.3-rc.1).

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
