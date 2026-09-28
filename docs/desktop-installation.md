# Next 桌面安装器切换与验收

`Next` 的 `make gui-build`、`make gui-test` 和 `make gui-installer` 已采用 Wails
宿主。公开 v0.2.4 仍使用 Swift 菜单栏宿主；本文件不表示已经发布新版本。

## 安装契约

| 项目 | Next 行为 |
| --- | --- |
| App 路径 / 身份 | `/Applications/OpenSurge.app` / `com.opensurge.menubar`，保持现有身份 |
| 主程序 | `OpenSurgeDesktop`；PKG 完整替换 bundle，移除旧 `OpenSurgeMenuBar` |
| 开发预览 | 独立的 `OpenSurge Desktop Preview.app` / `com.opensurge.desktop.preview` |
| 界面 | Wails + 系统 WebView，复用 React/TS；主窗口、菜单栏共享原有 Go API |
| 后台 | 用户 Control Service 与系统 root Helper 保持独立 |
| 首次窗口 | 请求 1440 × 900 points，适配屏幕，允许用户调整并记忆 |
| 启动 | 仅正式安装位置的 App 唤醒已有用户服务；不启动网关、不强制重启服务 |
| 登录项 | 使用原身份的 `SMAppService.mainAppService`；安装器不自动注册或注销 |
| 卸载 | 原有 root-owned 固定脚本，保留数据 / 彻底删除两种模式 |

升级前仍必须完成 DHCP 接管恢复。preinstall 先停止精确安装路径下的 Swift / Wails
宿主，再卸载用户 Control Service，随后使用**新包携带的恢复 CLI**清理已安装网关，
释放属于 OpenSurge 的睡眠接管，最后卸载 Helper。无法停止宿主、恢复未完成或网关清理
失败时拒绝继续。开发副本和 Preview 不属于升级进程处理范围。

Wails 接到 Installer 的 TERM 时只退出宿主，不显示交互退出确认，也不替 Installer 执行
网关清理。用户主动退出仍保留原生确认和最新状态门禁。postinstall 保留配置、来源、
凭据、策略与 runtime，安装后台服务并移除历史 `OpenSurge Menu Bar.app` 路径。

同一 bundle ID 不等于登录项迁移已经验证。签名身份、macOS 审批和系统登录项状态必须以
真实安装后的 `SMAppService` 返回值与重登录结果为准；不能凭 fixture 声称原登录项已继承。

## 本地构建

从明确提交的干净源码导出构建，保留既有 PKG。示例变量需替换为本次构建的实际来源：

```sh
OPENSURGE_VERSION=0.2.4 \
OPENSURGE_BUILD_NUMBER=1 \
OPENSURGE_RELEASE_TAG=v0.2.4-next \
OPENSURGE_APP_ARCH=arm64 \
OPENSURGE_MIHOMO_BINARY=/absolute/path/to/pinned/mihomo \
OPENSURGE_DNSMASQ_BINARY=/absolute/path/to/pinned/dnsmasq \
OPENSURGE_PKG_OUTPUT=/absolute/path/to/OpenSurge-0.2.4-next-arm64-build1-COMMIT-unsigned.pkg \
make gui-installer
```

构建使用临时 staging，不清空既有安装包；目标文件已存在时拒绝覆盖。
`OPENSURGE_VERSION` 必须匹配 release tag 的基础版本。省略 tag 的本地 Next 构建带
`-next`，正式发布流水线仍显式传入稳定版或 RC tag。Apple Silicon 与 Intel 必须分别提供
匹配架构的 mihomo / dnsmasq。

```sh
./scripts/verify-unsigned-gui-installer.sh PACKAGE 0.2.4 arm64 13.0 v0.2.4-next 1
shasum -a 256 PACKAGE > PACKAGE.sha256
shasum -a 256 -c PACKAGE.sha256
```

最后一个 verifier 参数可指定 build number。验证会检查生产 App 身份、名称、版本、主程序、
架构、最低 macOS 目标、App 签名完整性、无旧 Swift 程序、Helper 和 Wails 许可证。
本地默认 App 使用 ad-hoc 签名，PKG 未签名、未公证；不能描述为 Developer ID 发布包。

## 验收层级

| 门槛 | 能证明什么 | 不能证明什么 |
| --- | --- | --- |
| `make test` / `make web-test web-build` / `make desktop-test` | Go、React、桌面认证与能力边界回归 | 真实网络、安装或 macOS 版本兼容性 |
| `check-gui-packaging.sh` | 生产宿主选择、进程匹配、脚本顺序、数据保留与卸载范围 | 真实 launchd / PackageKit / 管理员授权 |
| 两架构 App / PKG 构建和解包验证 | arm64 / x86_64 payload 与 macOS 13 编译目标 | Intel 硬件或 macOS 13 的实际运行 |
| macOS 14 arm64 原生 fixture | WebView、窗口、标题栏、菜单、固定能力交互、TERM 退出 | 实际安装位置的身份门禁与 root 操作 |

`tests/packaging/test_installer.py` 仅在自己的临时目录运行脚本副本，将硬编码安装路径
重定位，并用假系统命令代替 launchd、权限变更、睡眠设置与 receipt 操作。它覆盖首次
seed、升级保留、恢复阻断、失败停止，以及保留数据 / 彻底卸载。该门槛不触碰已安装 App。

发布前仍需在可恢复的测试安装中完成以下操作；当前集成不能代替这些实机记录：

1. 从 Swift v0.2.4 升级及重复安装 Wails 包；确认单一 App、无旧进程/重复图标、数据保留。
2. 升级前分别设置登录项开启和关闭；升级后读取实际状态、重登录，确认不会意外启用或丢失。
3. 完整退出后重开 App，确认只唤醒 Control Service，网关保持停止。
4. 管理员授权取消、卸载保留数据后重装、彻底卸载；核对 LaunchAgent、Helper、receipt 和文件。
5. macOS 13 及 Intel 原生 GUI：系统 WebView、主窗口、物理菜单栏点击、登录项、退出与卸载。
6. 如果要宣称真实升级网络清理已验证，按 [validation gates](agent-wiki/wiki/concepts/validation-gates.md)
   执行相关 Lab 门槛，完成清理；fixture、截图和打包成功不代替这些证据。

## 旧宿主

`apps/menubar/`、旧构建和检查脚本保留供历史版本维护，已退出默认 GUI/发布链路。
显式 `make menubar-build` 输出到 `bin/legacy/OpenSurge.app`，不会覆盖 Wails 产物。
