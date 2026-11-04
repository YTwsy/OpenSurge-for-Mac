[简体中文](#v033-主要变化相对-v032) · [English](#english)

> **v0.3 series codename: Verdilion**

### v0.3.3 主要变化（相对 v0.3.2）

v0.3.3 升级 Mihomo 内核，改善网关异常后的恢复体验，新增可选的域名嗅探与配置来源删除，并让日常操作反馈和连接查看更清晰。

- **升级 Mihomo 内核至 1.19.33**：采用 OpenSurge-patched Mihomo `1.19.33-opensurge.1`，纳入上游 TLS、DoH 查询及配置重载时 fake-IP 分配状态的修复，同时保留 OpenSurge 的下游 IPv6 接入与本机 IPv6 独立配置能力。见[上游更新说明](https://github.com/MetaCubeX/mihomo/releases/tag/v1.19.33)。
- **修复 DNS 恢复错误导致网关无法继续启动的问题**：系统 DNS 原设置独立保存，停止、回滚和重启代理引擎时按备份恢复。原网络服务消失后跳过旧记录，恢复失败时保留提醒与备份，并继续清理其他网关资源。（[PR #109](https://github.com/YTwsy/OpenSurge-for-Mac/pull/109)；[#107](https://github.com/YTwsy/OpenSurge-for-Mac/issues/107) — Thanks @yizao）
- **配置有误时仍可执行恢复操作**：停止网关、DHCP 恢复和状态诊断不再被无关的部署参数或未应用策略阻断。租约读取失败单独提示；仅重启代理引擎时，也不再要求无关的 DHCP 依赖完整可用。应用新配置仍执行完整校验。见 [PR #109](https://github.com/YTwsy/OpenSurge-for-Mac/pull/109)。
- **新增可选的 Sniffer 域名嗅探**：在“网络设置 → 高级 Mihomo / DNS 设置”中按需启用，默认关闭。当外部加密 DNS 或缓存的真实 IP 导致连接缺少域名映射时，可从 HTTP Host、TLS/QUIC 握手中补充可识别的域名供规则匹配。连接页同步展示嗅探域名，原始目标 IP 与设备规则优先级保持不变。（[PR #111](https://github.com/YTwsy/OpenSurge-for-Mac/pull/111)；[#110](https://github.com/YTwsy/OpenSurge-for-Mac/issues/110) — Thanks @yizao）
- **支持删除不再使用的代理与规则源**：来源卡片新增“删除来源”，清理列表记录、托管历史快照和保存的订阅地址；即使文件已在 Finder 中删除，也能移除残留记录。原始 YAML、导出副本和全局附加配置保留。当前选中或仍被引用的来源需先解除使用，删除操作不会重载网关。（[#105](https://github.com/YTwsy/OpenSurge-for-Mac/issues/105) — Thanks @Tommytalk）
- **统一操作通知与配置检查反馈**：导入、刷新、保存和网络操作使用悬浮通知，减少页面跳动。成功提示自动收起，错误保留供查看、复制和关闭；多条配置问题汇总为可展开摘要，DNS 恢复等子步骤的提醒也会独立保留。见 [PR #108](https://github.com/YTwsy/OpenSurge-for-Mac/pull/108)、[PR #109](https://github.com/YTwsy/OpenSurge-for-Mac/pull/109)。
- **改善网关停止时的连接页与本机策略布局**：正常停止后显示“尚未启动网络模式”和网络设置入口，保留已知设备信息；重新启动后刷新连接，避免沿用上次运行的数据。本机出口与“仅影响本机”标记的对齐及窄屏布局也更清晰。
- **滚动连接列表时随时切换设备**：设备筛选列表保持可见，较长列表可独立滚动；手机等窄屏保留横向滑动选择。设备页“前往网络设置”按钮同步统一外观与间距。

### 升级说明

- Sniffer 默认关闭；未配置该选项的旧配置保持关闭，已有设置会保留，导入订阅不会覆盖该开关。修改时需停止网关，保存网络配置后重新启动；它不能保证识别 ECH 隐藏的真实域名或其他没有可识别域名的流量。
- Mac 系统 DNS 按接管前备份恢复；接管期间由用户或其他软件修改的 DNS 也会被原备份替换。恢复失败的提醒和已保存的备份会保留供后续重试。
- 下游 IPv6 仍为实验性功能。本次内核升级已在 Apple Silicon 与受控虚拟客户端环境完成基础网络、真实 TUN 和三个下游 IPv6 拓扑的回归；这些结果不覆盖物理客户端、真实 Tailnet 或 Intel 硬件。完整范围见[网络回归记录](https://github.com/YTwsy/OpenSurge-for-Mac/blob/v0.3.3/docs/agent-wiki/tasks/finished_archived/mihomo-1.19.33-network-regression.md)。
- 完整变更：[v0.3.2...v0.3.3](https://github.com/YTwsy/OpenSurge-for-Mac/compare/v0.3.2...v0.3.3)。

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

- OpenSurge-patched Mihomo `1.19.33-opensurge.1` 的上游基线源码：<https://github.com/MetaCubeX/mihomo/tree/3f27ef39f8c96bbfbc8b0b9bb10f5c9ef60149d8>
- dnsmasq 2.93 源码：<https://thekelleys.org.uk/dnsmasq/dnsmasq-2.93.tar.gz>

---

## English

### v0.3.3 highlights since v0.3.2

v0.3.3 upgrades Mihomo, improves recovery after gateway interruptions, adds optional domain sniffing and configuration source deletion, and makes everyday feedback and connection browsing clearer.

- **Upgrade Mihomo to 1.19.33:** Use OpenSurge-patched Mihomo `1.19.33-opensurge.1`, incorporating upstream TLS, DoH query and fake-IP allocation-state fixes during configuration reload, while retaining OpenSurge's downstream IPv6 ingress and independent host IPv6 configuration. See the [upstream release notes](https://github.com/MetaCubeX/mihomo/releases/tag/v1.19.33).
- **Fix DNS restoration errors blocking subsequent gateway startup:** Original system DNS settings are saved independently and restored during stop, rollback and proxy-engine restart. Records for network services that no longer exist are skipped. Restoration failures retain a warning and backup for retry while cleanup of other gateway resources continues. ([PR #109](https://github.com/YTwsy/OpenSurge-for-Mac/pull/109); [#107](https://github.com/YTwsy/OpenSurge-for-Mac/issues/107) — Thanks @yizao)
- **Keep recovery actions available when configuration is invalid:** Gateway stop, DHCP recovery and status diagnostics are no longer blocked by unrelated deployment settings or unapplied policies. Lease-reading errors are reported separately, and restarting only the proxy engine no longer requires unrelated DHCP dependencies. Applying a new configuration still performs full validation. See [PR #109](https://github.com/YTwsy/OpenSurge-for-Mac/pull/109).
- **Add optional Sniffer domain detection:** Enable it when needed in Network Settings → Advanced Mihomo / DNS settings; it is off by default. When external encrypted DNS or cached real IPs leave a connection without a domain mapping, sniffing can supplement domain rules with recognizable hostnames from HTTP Host and TLS/QUIC handshakes. Connections displays sniffed domains while preserving the original destination IP and device-rule precedence. ([PR #111](https://github.com/YTwsy/OpenSurge-for-Mac/pull/111); [#110](https://github.com/YTwsy/OpenSurge-for-Mac/issues/110) — Thanks @yizao)
- **Delete unused proxy and rule sources:** Source cards now offer Delete Source to remove the library entry, managed historical snapshots and saved subscription URL, including stale entries whose files were already deleted in Finder. Original YAML files, exported copies and the Global Profile Overlay are retained. Selected or referenced sources must be released from use first; deletion does not reload the gateway. ([#105](https://github.com/YTwsy/OpenSurge-for-Mac/issues/105) — Thanks @Tommytalk)
- **Unify operation notifications and configuration checks:** Imports, refreshes, saves and network operations use floating notifications to reduce layout shifts. Success messages dismiss automatically; errors remain available for reading, copying and dismissal. Multiple configuration issues are grouped into expandable summaries, and warnings from individual steps such as DNS restoration remain visible independently. See [PR #108](https://github.com/YTwsy/OpenSurge-for-Mac/pull/108), [PR #109](https://github.com/YTwsy/OpenSurge-for-Mac/pull/109).
- **Improve connection guidance while stopped and local policy layout:** A normally stopped gateway shows startup guidance and a link to Network Settings while retaining known device identities. Starting again refreshes connections without reusing samples from the previous run. Local egress controls and the Mac-only badge also have clearer alignment and narrow-screen layout.
- **Switch devices while scrolling through connections:** The device filter stays visible, longer device lists scroll independently, and phones retain horizontal device selection. The Devices page's Go to Network Settings button also uses consistent styling and spacing.

### Upgrade notes

- Sniffer defaults to off when the setting is absent, and existing settings are preserved. Imported subscriptions cannot override the switch. Stop the gateway, save the changed network configuration, and start it again to apply. Sniffing cannot reliably identify the real domain hidden by ECH or traffic without a recognizable hostname.
- Mac system DNS is restored from the original pre-takeover backup, replacing DNS changes made by the user or other software during takeover. Failed restoration retains a warning and the persisted backup for retry.
- Downstream IPv6 remains experimental. This core upgrade has passed basic networking, real TUN and all three downstream IPv6 Virtual Lab topologies on Apple Silicon with controlled virtual clients. These results do not cover physical clients, real Tailnet traffic or Intel hardware. See the [network regression record](https://github.com/YTwsy/OpenSurge-for-Mac/blob/v0.3.3/docs/agent-wiki/tasks/finished_archived/mihomo-1.19.33-network-regression.md) for the full scope.
- Full changelog: [v0.3.2...v0.3.3](https://github.com/YTwsy/OpenSurge-for-Mac/compare/v0.3.2...v0.3.3).

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

- Upstream baseline source for OpenSurge-patched Mihomo `1.19.33-opensurge.1`: <https://github.com/MetaCubeX/mihomo/tree/3f27ef39f8c96bbfbc8b0b9bb10f5c9ef60149d8>
- dnsmasq 2.93 source: <https://thekelleys.org.uk/dnsmasq/dnsmasq-2.93.tar.gz>
