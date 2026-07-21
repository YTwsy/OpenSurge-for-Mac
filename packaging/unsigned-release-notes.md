[简体中文](#简体中文) · [English](#english)

## 简体中文

### 主要变化

<!-- 发布前请更新为当前版本的主要变化，并同步维护 English / Highlights。 -->

- 大幅改进网络设置的操作流程和引导：新增网关模式说明与拓扑图，统一网关启动、停止和恢复入口，并自动定位到接下来的操作位置。
- 更清楚地区分旁路由（手工网关）、局域网 DHCP 接管和独立下游 LAN 三种模式，帮助用户选择适合自己的网络拓扑。
- 增强局域网 DHCP 接管流程的可靠性：切换 Mac 固定 IPv4 后会回读并确认网络配置，避免在设置尚未生效时继续操作。
- 活跃使用 Web GUI 时自动续期会话；会话过期后会明确提示从菜单栏重新打开 OpenSurge。
- 优化菜单栏图标和应用名称显示，并补充中英文 App 使用指南。

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

- Significantly improved the Network Settings workflow and guidance with clearer gateway-mode descriptions, topology diagrams, unified gateway controls, and automatic navigation to the next action.
- More clearly distinguishes between Bypass Router (Manual Gateway), LAN DHCP Takeover, and Separate Downstream LAN modes.
- Improved the reliability of LAN DHCP Takeover by confirming that the Mac's fixed IPv4 configuration has actually been applied before continuing.
- Active Web GUI sessions now renew automatically. When a session expires, OpenSurge clearly explains how to reopen the Web GUI from the menu bar.
- Refined the menu bar icon and application naming, and added App guides in both Chinese and English.

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
