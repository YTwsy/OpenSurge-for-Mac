[简体中文](#简体中文) · [English](#english)

## 简体中文

### 主要变化

<!-- 发布前请更新为当前版本的主要变化，并同步维护 English / Highlights。 -->

- 修复 Mac 重启后旧 runtime 阻塞再次启动的问题（Issue #22）：OpenSurge 现在记录开机 session 与 dnsmasq/mihomo 进程指纹，将上一次开机留下的状态明确标记为「重启后待清理」。控制面提供「安全清理旧状态」，不会向旧 PID 发送信号，也不会改动本次开机的 PF 或 IPv4 forwarding；若上次运行启用了本机系统代理协同，则恢复 OpenSurge 启动前保存的 HTTP/HTTPS 状态。
- 修复 PKG 升级偶发需要安装两次：preinstall 会先停止可能重新唤醒 Control Service 的菜单栏 App，持续卸载精确的用户服务，并使用新安装包内置的当前版本恢复 CLI 清理网关，不再依赖正在被替换的旧版 `omg stop`。
- 改进睡眠唤醒与 Wi-Fi 重连恢复（Issue #23）：只有本次开机的网关 runtime 仍有效、且 Mihomo 已停止或本地 controller 明确拒绝连接时，连通性页才显示「恢复 Mihomo」；健康运行和完整重启后的中断状态不会显示该入口。
- 优化设备出口卡片：设备 ID、IPv4 与 MAC 统一为紧凑的绿色代码标签并使用适合中文的字体；未登记 MAC 与「策略已暂停」合并显示在 MAC 位置，不再额外展示突兀的棕色警告块。

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

- Fixed stale runtime state blocking startup after a Mac reboot (Issue #22): OpenSurge now records the boot session and dnsmasq/mihomo process fingerprints, and clearly marks state left by an earlier boot as **Interrupted after restart**. The control plane provides **Safely clear old state** without signaling stale PIDs or changing PF or IPv4 forwarding for the current boot. If local system-proxy coordination was active, cleanup restores the saved pre-OpenSurge HTTP/HTTPS state.
- Fixed an intermittent PKG upgrade failure that required a second installation attempt: preinstall now stops the menu bar app that can wake the Control Service, repeatedly unloads the exact user service, and uses the current recovery CLI embedded in the new package instead of relying on the old `omg stop` being replaced.
- Improved wake and Wi-Fi reconnect recovery (Issue #23): Connectivity shows **Recover Mihomo** only when the current-boot gateway runtime is still valid and Mihomo has stopped or its local controller explicitly refuses connections. Healthy runtimes and runtimes interrupted by a full reboot do not show the action.
- Refined device outlet cards: device ID, IPv4, and MAC now use consistent compact green code tags with Chinese-friendly typography. Missing MAC and **policy paused** status are shown together in the MAC slot instead of a separate prominent brown warning block.

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
