[简体中文](#简体中文) · [English](#english)

## 简体中文

### 主要变化

<!-- 发布前请更新为当前版本的主要变化，并同步维护 English / Highlights。 -->

- 菜单栏新增原生“卸载 OpenSurge”流程：网关停止后可通过 macOS 管理员授权移除 App、Control Service 与 root Helper，并选择保留配置数据或彻底删除。
- 菜单栏 App 改用纯 AppKit 生命周期，避免系统创建或恢复无用的空设置窗口，并提升应用启动与状态面板行为的稳定性。
- 修复 Issue #7：导入的 mihomo profile 现在兼容 flow/block 风格的 `rules`、策略组与 rule providers，以及带引号键、注释和相对 Provider 路径。
- 来源预览与实际应用现在复用同一套 YAML 结构验证；无效候选配置会在写入或重载前失败，保持当前网关和已应用配置不变。

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

- Added a native Uninstall OpenSurge flow to the menu bar. Once the gateway is stopped, administrator authorization can remove the app, Control Service, and root Helper while either preserving configuration data or deleting everything.
- Moved the menu bar app to a pure AppKit lifecycle, preventing macOS from creating or restoring an unused empty Settings window and improving launch and panel stability.
- Fixed Issue #7: imported mihomo profiles now support flow- and block-style `rules`, proxy groups, and rule providers, including quoted keys, comments, and relative provider paths.
- Source preview and final application now share the same YAML structural validation. Invalid candidates fail before write or reload, leaving the current gateway and applied configuration unchanged.

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
