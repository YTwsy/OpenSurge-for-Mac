> **Unsigned stable release / 未签名正式版本**
>
> These packages do not use an Apple Developer ID and are not notarized. They install
> a root helper and can change DHCP, DNS, PF, IPv4 forwarding, and TUN state. Install
> them only if you trust this repository and understand the gateway recovery procedure.
>
> 这些安装包没有 Apple Developer ID 签名，也未经过 notarization。它们会安装 root
> helper，并能够修改 DHCP、DNS、PF、IPv4 forwarding 与 TUN 状态。请仅在信任本仓库并
> 理解网关恢复流程时安装。

## Highlights / 主要变化

- Publish one stable GitHub Release with separate Apple Silicon and Intel installers.
- 为 Apple Silicon 与 Intel Mac 在同一个正式 GitHub Release 中提供独立安装包。

## Choose a package / 选择安装包

| Mac | Package / 安装包 | Minimum system / 最低系统 |
| --- | --- | --- |
| Apple Silicon (M1 or newer) | `arm64-unsigned.pkg` | macOS 13+ |
| Intel | `x86_64-unsigned.pkg` | macOS 13+ |

Both packages pass payload, Mach-O architecture, and minimum-macOS checks in the release
workflow. The Intel package is cross-built on Apple Silicon; this release does not claim that
`make lab-test` or `make lab-test-tun` ran on physical Intel hardware.

发布工作流会检查两个 pkg 的 payload、Mach-O 架构与最低 macOS 版本。Intel pkg 在 Apple
Silicon runner 上交叉构建；本版本不宣称已在实体 Intel Mac 上运行 `make lab-test` 或
`make lab-test-tun`。

## Install / 安装

1. Download the package matching your Mac and `SHA256SUMS`.
2. Optionally verify all downloaded packages with `shasum -a 256 -c SHA256SUMS` and
   verify the selected package's GitHub build provenance with
   `gh attestation verify OpenSurge-for-Mac-*-arm64-unsigned.pkg -R YTwsy/OpenSurge-for-Mac`
   or `gh attestation verify OpenSurge-for-Mac-*-x86_64-unsigned.pkg -R YTwsy/OpenSurge-for-Mac`.
3. Double-click the package. When Gatekeeper blocks it, open **System Settings →
   Privacy & Security**, choose **Open Anyway**, authenticate, and reopen the package.
4. Finish Installer with an administrator account, then open
   **OpenSurge** from `/Applications`.

1. 下载与 Mac 芯片匹配的安装包及 `SHA256SUMS`。
2. 可选：运行 `shasum -a 256 -c SHA256SUMS` 校验已下载的安装包，并使用
   `gh attestation verify OpenSurge-for-Mac-*-arm64-unsigned.pkg -R YTwsy/OpenSurge-for-Mac`
   或 `gh attestation verify OpenSurge-for-Mac-*-x86_64-unsigned.pkg -R YTwsy/OpenSurge-for-Mac`
   核对所选安装包的 GitHub 构建来源。
3. 双击安装包；被 Gatekeeper 阻止后，进入**系统设置 → 隐私与安全性**，选择
   **仍要打开**、完成身份验证，然后重新打开安装包。
4. 使用管理员账户完成 Installer，随后从 `/Applications` 打开
   **OpenSurge**。

Do not disable Gatekeeper globally or remove quarantine recursively. The installed
gateway remains stopped until you explicitly start it from the OpenSurge control plane.

不要全局关闭 Gatekeeper，也不要递归删除 quarantine。安装完成后网关仍保持停止，只有在
OpenSurge 控制面中明确操作才会启动。

## License / 许可证

OpenSurge original code is licensed under `GPL-3.0-only`. Third-party license
texts, notices, and exact corresponding-source links are installed under
`/Library/Application Support/OpenSurge/share/licenses/`.

- mihomo 1.19.27 source: <https://github.com/MetaCubeX/mihomo/tree/5184081ac327394d9e15fa5d5f9f4a61e723fd94>
- dnsmasq 2.93 source: <https://thekelleys.org.uk/dnsmasq/dnsmasq-2.93.tar.gz>

OpenSurge 自有代码采用 `GPL-3.0-only`。第三方许可证、声明与准确的对应源码链接会安装到
`/Library/Application Support/OpenSurge/share/licenses/`。
