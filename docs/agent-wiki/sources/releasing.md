# 桌面构建与发布

本页维护可复现本地构建、产物校验、版本身份和签名方式。安装行为见 [分发契约](distribution.md)，实际安装与平台结论见 [证据入口](validation/evidence-map.md#桌面与安装)。

## 本地构建

从明确提交的干净源码导出构建，保留既有 PKG。示例变量需替换为本次构建的实际来源：

```sh
OPENSURGE_VERSION=0.3.2 \
OPENSURGE_BUILD_NUMBER=1 \
OPENSURGE_RELEASE_TAG=v0.3.2 \
OPENSURGE_APP_ARCH=arm64 \
OPENSURGE_MIHOMO_CACHE_DIR=/absolute/path/to/shared/mihomo-builds \
OPENSURGE_DNSMASQ_BINARY=/absolute/path/to/pinned/dnsmasq \
OPENSURGE_PKG_OUTPUT=/absolute/path/to/OpenSurge-0.3.2-arm64-build1-COMMIT-unsigned.pkg \
make gui-installer
```

构建使用临时 staging，不清空既有安装包；目标文件已存在时拒绝覆盖。
`OPENSURGE_VERSION` 必须匹配 release tag 的基础版本。默认 tag 为当前正式版；
发布流水线显式传入稳定版或 RC tag。Apple Silicon 与 Intel 必须分别提供
匹配架构的 dnsmasq；mihomo 按目标架构校验缓存或重新构建。

每次打包先校验固定上游源码的 SHA-256，再按源码、当前补丁与 overlay 的文件名及
内容、构建脚本、Go 工具链、目标架构、版本、构建参数生成缓存键。缓存中的来源记录
必须完全匹配，且可执行文件摘要必须一致，才复制到本次 staging；否则重新构建、
通过核心源码测试后发布缓存。构建期间输入变化或测试/编译失败不得发布缓存。
命中缓存仍检查实际入包核心的 IPv6 四组合；本机缓存记录不是签名或远程供应链认证。

默认缓存为 `runtime/release-tools/cache/mihomo-builds`。可设置
`OPENSURGE_MIHOMO_CACHE_DIR=/absolute/path/to/mihomo-builds`，让干净源码导出和多个
checkout 共用缓存；与工作区路径无关的输入完全一致时可命中。不同架构/输入的条目
分别保留，Go 编译缓存也可继续使用。引用外部文件/程序的 `GOFLAGS`（`-overlay`、
`-modfile`、`-toolexec`）走重新构建、不缓存路径。

仍拒绝裸二进制覆盖参数 `OPENSURGE_MIHOMO_BINARY`：同版本号或无来源记录的旧核心
不能证明包含当前补丁，首次使用新缓存流程需重建。可用
`OPENSURGE_MIHOMO_SOURCE_ARCHIVE=/absolute/path/to/mihomo-source.tar.gz`
复用通过固定摘要校验的源码归档。

```sh
./scripts/verify-unsigned-gui-installer.sh PACKAGE 0.3.2 arm64 13.0 v0.3.2 1
shasum -a 256 PACKAGE > PACKAGE.sha256
shasum -a 256 -c PACKAGE.sha256
```

最后一个 verifier 参数可指定 build number。验证会检查生产 App 身份、名称、版本、主程序、
架构、最低 macOS 目标、App 签名完整性、无旧 Swift 程序、Helper 和 Wails 许可证。
本地默认 App 使用 ad-hoc 签名，PKG 未签名、未公证；不能描述为 Developer ID 发布包。

## 版本与代号

`packaging/release-codenames.json` is the series-codename source for the current
Web/desktop/tray UI, native bundle metadata and GitHub Release title. Unknown
series display the version without inheriting a previous codename. The retained
Swift host is the v0.2 maintenance reference and is not included in v0.3 packages.
`OPENSURGE_RELEASE_TAG` carries the full stable or candidate tag; the macOS numeric bundle
and package version use its base version, such as `0.3.0` for `v0.3.0-rc.1`.
Update discovery accepts stable releases only; RC-to-RC updates require a manual download.

`OPENSURGE_VERSION` 写入 PKG receipt 与 App short version，`OPENSURGE_BUILD_NUMBER` 写入 `CFBundleVersion`，完整 tag 写入 `OpenSurgeReleaseTag`。stable/RC 的基础版本必须一致；原生更新检查只接受 stable。

## 签名与公开产物

有 Apple Developer 身份时，可设置 `OPENSURGE_CODESIGN_IDENTITY`、`OPENSURGE_INSTALLER_IDENTITY` 构建，再使用已通过 `notarytool store-credentials` 配置的 `OPENSURGE_NOTARY_PROFILE`：

```sh
make gui-notarize PKG=/absolute/path/to/PACKAGE.pkg
```

缺少该身份的 tag workflow 发布 `arm64-unsigned.pkg` 与 `x86_64-unsigned.pkg`，并提供合并 SHA-256 清单和每个 PKG 的 GitHub artifact attestation。GitHub 正式 Release 或 attestation 不证明 Developer ID 签名、公证或 Gatekeeper 放行；用户安装按单包“仍要打开”流程，不要求全局关闭 Gatekeeper 或递归删除 quarantine。

历史 v0.3.0 的 `Next` / release 分支合入流程见 [迁移记录](../tasks/finished_archived/desktop-migration-v0.3.md)。后续发布须以本次明确的版本、来源提交和维护者约定为准。
