[简体中文](#简体中文) · [English](#english)

> **v0.2 系列代号：Wind Rose**<br>
> **v0.2 series codename: Wind Rose**

## 简体中文

### 相对 v0.2.0 的主要变化

- **更准确的局域网与设备管理**：新增真实子网前缀配置，不再默认所有网络都是 `/24`。迁移网络后，旧网段设备会保留为休眠登记，但不会进入 DHCP、运行时策略、selector 或 IPv6 设备映射；设备卡现在可直接修改登记信息或删除设备。
- **更清晰的规则与策略控制**：Web GUI 新增规则库，将规则集、无出口分流模板和设备出口分开管理，并提供一份默认不应用的 Claude Code 社区示例。策略页新增分组导航，便于浏览较长的代理组列表。
- **按范围刷新活动连接**：切换 Mac 本机模式或设备出口后，可以只关闭对应的活动连接，让新连接立即使用当前策略，而不影响其他设备或无关流量。
- **fake-IP 映射持久化**：新增默认开启的 `mihomo.store_fake_ip`，在 apply/restart 后恢复已有 fake-IP 映射，减少长驻进程继续使用失效地址的问题。该设置不会保留既有 TCP/QUIC 连接。
- **更严格的可复现验证**：IPv6 Virtual Lab 现在验证无 TCP/HTTP2 fallback 的真实 HTTP/3-only 请求，覆盖 `DIRECT`、受控 SOCKS5 UDP 出口和 HTTP-only 出口 fail-closed；设备策略 Lab 同时覆盖非 `/24` 网络、旧网段设备休眠和设备级连接刷新。结论不扩展到所有 QUIC 版本、连接迁移或公网代理组合。

### 选择安装包

| Mac 类型 | 安装包 | 最低系统 |
| --- | --- | --- |
| Apple Silicon（M1 及更新芯片） | `arm64-unsigned.pkg` | macOS 13+ |
| Intel Mac | `x86_64-unsigned.pkg` | macOS 13+ |

> 安装包未进行 Developer ID 签名或 notarization。正式 Release 会同时提供 `SHA256SUMS` 和 GitHub build provenance，供下载后核验。

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

- OpenSurge-patched Mihomo `1.19.27-opensurge.1` 的上游基线源码：<https://github.com/MetaCubeX/mihomo/tree/5184081ac327394d9e15fa5d5f9f4a61e723fd94>
- dnsmasq 2.93 源码：<https://thekelleys.org.uk/dnsmasq/dnsmasq-2.93.tar.gz>

---

## English

### Highlights since v0.2.0

- **Accurate LAN and device scoping:** OpenSurge now supports the real downstream prefix instead of assuming `/24`. Registrations outside the current LAN remain editable but stay dormant and are excluded from DHCP, runtime policies, selectors, and IPv6 identity mappings.
- **Clearer rule and policy controls:** The Web GUI rule library separates rule sets, outlet-free routing templates, and per-device egress. Policy-group navigation makes large imported profiles easier to browse.
- **Scoped connection refresh:** After changing the local-Mac mode or a device egress, only matching active connections can be closed so new connections use the current selection without interrupting unrelated devices.
- **Persistent fake-IP mappings:** The new default `mihomo.store_fake_ip` setting restores mappings across apply/restart. It does not preserve existing TCP or QUIC sessions.
- **Stronger reproducible validation:** The IPv6 Lab now performs real HTTP/3-only requests without TCP/HTTP2 fallback through controlled `DIRECT`, SOCKS5 UDP, and HTTP-only fail-closed paths. Device-policy coverage also includes non-`/24` LANs, dormant off-LAN devices, and scoped connection refresh.

### Choose a package

| Mac | Package | Minimum system |
| --- | --- | --- |
| Apple Silicon (M1 or newer) | `arm64-unsigned.pkg` | macOS 13+ |
| Intel Mac | `x86_64-unsigned.pkg` | macOS 13+ |

> The installers are not Developer ID signed or notarized. The stable Release will also provide `SHA256SUMS` and GitHub build provenance for post-download verification.

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

- Upstream baseline source for OpenSurge-patched Mihomo `1.19.27-opensurge.1`: <https://github.com/MetaCubeX/mihomo/tree/5184081ac327394d9e15fa5d5f9f4a61e723fd94>
- dnsmasq 2.93 source: <https://thekelleys.org.uk/dnsmasq/dnsmasq-2.93.tar.gz>
