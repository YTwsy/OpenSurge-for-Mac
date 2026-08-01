[简体中文](#简体中文) · [English](#english)

## 简体中文

### 主要变化

<!-- 发布前请更新为当前版本的主要变化，并同步维护 English / Highlights。 -->

- 新增「这台 Mac 的出口方式」控制：网关运行期间可在控制面为本机公网流量切换 **按规则 / 固定出口 / 本机直连** 三种模式，并展示当前全局出口。
- 菜单栏新增轻量版本发现：打开面板后自动检查最新稳定版，也可手动刷新；发现新版本时只提供打开对应 GitHub Release 下载页的按钮，不自动下载或安装 PKG。RC 构建保留完整预发布 tag，不会降级到旧稳定版，同版本正式版发布后仍会提示。
- 新增默认关闭的「Mac 本机系统代理协同」：TUN 模式下可同时启用 macOS HTTP/HTTPS 系统代理，兼容 SafeDNS、DNS Proxy 等 Network Extension 干扰 TUN-only 本机 DNS 的场景；启动冲突会 fail closed，停止、回滚与 mihomo 重启失败时恢复原状态；Desired 网络配置中的本机系统代理与每设备策略改用紧凑的状态开关。
- 旁路由模式允许只登记固定 IPv4，MAC 作为可选身份信息；切换到 DHCP 接管时可确认当前唯一观测到的 MAC，暂时无法识别的设备会保留但暂停策略，补充 MAC 后恢复。已登记设备仍可根据同 MAC 的唯一观测结果安全更新 IPv4；歧义或冲突证据保持 fail closed。
- TUN 启动异常检查：网关现在会校验 TUN 就绪状态并处理 DHCP 放弃（DHCP abandonment），加固就绪恢复路径，容忍更慢的运行时状态读取；移除投机式 TUN 预检，将预检限定到 DHCP 规划。

### 选择安装包

| Mac 类型 | 安装包 | 最低系统 |
| --- | --- | --- |
| Apple Silicon（M1 及更新芯片） | `arm64-unsigned.pkg` | macOS 13+ |
| Intel Mac | `x86_64-unsigned.pkg` | macOS 13+ |

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

- mihomo 1.19.27 源码：<https://github.com/MetaCubeX/mihomo/tree/5184081ac327394d9e15fa5d5f9f4a61e723fd94>
- dnsmasq 2.93 源码：<https://thekelleys.org.uk/dnsmasq/dnsmasq-2.93.tar.gz>

---

## English

### Highlights

- Added a "this Mac" routing control: while the gateway is running, the control plane can switch the local Mac's public traffic between **rule-based / fixed-outlet / direct** modes and shows the current global outlet.
- Added lightweight update discovery to the menu bar: the panel checks for the latest stable release automatically and supports manual refresh; when a newer version exists it only opens that GitHub Release download page and does not download or install the PKG. RC builds retain their full prerelease tag, avoid older stable downgrades, and still discover the stable release with the same base version.
- Added an off-by-default local system-proxy coordination option: in TUN mode OpenSurge can enable macOS HTTP and HTTPS proxies together for SafeDNS, DNS Proxy, and similar Network Extension conflicts; startup fails closed on existing proxy configuration, and stop, rollback, or failed mihomo restart restores the previous state; the local system-proxy and per-device-policy controls now use compact status switches in Desired Network Configuration.
- Bypass-router mode now allows registration by fixed IPv4 alone, with MAC as optional identity metadata. When switching to DHCP takeover, a uniquely observed current MAC can be confirmed; unresolved devices are preserved with their policies paused until a MAC is added. Existing registrations can still update IPv4 from a unique same-MAC observation, while ambiguous or conflicting evidence remains fail closed.
- TUN startup anomaly checks: the gateway now verifies TUN readiness and handles DHCP abandonment, hardens the readiness recovery path, tolerates slower runtime state reads, and drops speculative TUN preflight (preflight is now limited to DHCP planning).

### Choose a package

| Mac | Package | Minimum system |
| --- | --- | --- |
| Apple Silicon (M1 or newer) | `arm64-unsigned.pkg` | macOS 13+ |
| Intel Mac | `x86_64-unsigned.pkg` | macOS 13+ |

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

- mihomo 1.19.27 source: <https://github.com/MetaCubeX/mihomo/tree/5184081ac327394d9e15fa5d5f9f4a61e723fd94>
- dnsmasq 2.93 source: <https://thekelleys.org.uk/dnsmasq/dnsmasq-2.93.tar.gz>
