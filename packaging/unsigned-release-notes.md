> **Unsigned Apple Silicon preview / 未签名 Apple Silicon 预发布**
>
> This package does not use an Apple Developer ID and is not notarized. It installs a
> root helper and can change DHCP, DNS, PF, IPv4 forwarding, and TUN state. Install it
> only if you trust this repository and understand the gateway recovery procedure.
>
> 此安装包没有 Apple Developer ID 签名，也未经过 notarization。它会安装 root helper，
> 并能够修改 DHCP、DNS、PF、IPv4 forwarding 与 TUN 状态。请仅在信任本仓库并理解网关
> 恢复流程时安装。

## Install / 安装

1. Download the `arm64-unsigned.pkg` asset and `SHA256SUMS`.
2. Optionally verify the checksum with `shasum -a 256 -c SHA256SUMS` and the
   GitHub build provenance with
   `gh attestation verify OpenSurge-for-Mac-*-arm64-unsigned.pkg -R YTwsy/OpenSurge-for-Mac`.
3. Double-click the package. When Gatekeeper blocks it, open **System Settings →
   Privacy & Security**, choose **Open Anyway**, authenticate, and reopen the package.
4. Finish Installer with an administrator account, then open
   **OpenSurge Menu Bar** from `/Applications`.

1. 下载 `arm64-unsigned.pkg` 与 `SHA256SUMS`。
2. 可选：运行 `shasum -a 256 -c SHA256SUMS` 校验文件，并使用
   `gh attestation verify OpenSurge-for-Mac-*-arm64-unsigned.pkg -R YTwsy/OpenSurge-for-Mac`
   核对 GitHub 构建来源。
3. 双击安装包；被 Gatekeeper 阻止后，进入**系统设置 → 隐私与安全性**，选择
   **仍要打开**、完成身份验证，然后重新打开安装包。
4. 使用管理员账户完成 Installer，随后从 `/Applications` 打开
   **OpenSurge Menu Bar**。

Do not disable Gatekeeper globally or remove quarantine recursively. The installed
gateway remains stopped until you explicitly start it from the OpenSurge control plane.

不要全局关闭 Gatekeeper，也不要递归删除 quarantine。安装完成后网关仍保持停止，只有在
OpenSurge 控制面中明确操作才会启动。
