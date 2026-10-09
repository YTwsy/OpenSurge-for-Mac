# 桌面分发与安装契约

本页维护安装身份、文件归属、升级与卸载的边界；构建命令见 [构建与发布](releasing.md)，平台通过范围见 [证据入口](validation/evidence-map.md#桌面与安装)。

从 v0.3.0 起，`make gui-build`、`make gui-test` 和 `make gui-installer` 使用 Wails
宿主，替代 v0.2.4 的 Swift 菜单栏宿主。版本发布不扩大下文记录的验收范围。

## 安装契约

| 项目 | v0.3 行为 |
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

Wails 接到 Installer 的 TERM 时只退出宿主，不显示交互退出确认，也不替 Installer 执行
网关清理。用户主动退出仍保留原生确认和最新状态门禁。postinstall 保留配置、来源、
凭据、策略与 runtime，安装后台服务并移除历史 `OpenSurge Menu Bar.app` 路径。

同一 bundle ID 不等于登录项迁移已经验证。签名身份、macOS 审批和系统登录项状态必须以
真实安装后的 `SMAppService` 返回值与重登录结果为准；不能凭 fixture 声称原登录项已继承。

## 文件归属与升级顺序

PKG 固定以 `/` 为 install location，`packaging/gui-components.plist` 将 App 标为不可 relocatable，避免 Installer 把它放回构建工作区的 `payload/Applications`。应用、CLI、用户 Control Service、root Helper 与 Web 静态资源随包分发。

applied config、mihomo/dnsmasq、runtime 与 broker 位于 root-owned 的 `/Library/Application Support/OpenSurge`，Helper 位于 `/Library/PrivilegedHelperTools`。Control Service 经 admin 组只读访问 applied 状态，系统写入使用 Helper 的固定动作。

preinstall 的 recovery 门禁只接受 `idle`、`complete`、`complete_static`。先停止精确安装路径中的 Swift / Wails 宿主，再循环 bootout 用户 Control Service 并重扫可信 PID；等待中迟到的 bootstrap 也要清理。开发副本和 Preview 不属于升级进程处理范围。随后使用**新包脚本目录携带的** `omg-recovery stop` 处理旧 runtime，按 ownership marker 释放睡眠接管，最后 bootout Helper。无法停止宿主、恢复未完成或网关清理失败时，都在覆盖 payload 前终止。

postinstall 只在首次安装 seed managed 配置，不携带工作区 profile/device-policy 路径；安装版随后按设备策略契约初始化自己的空策略文件。升级保留 config、来源、凭据、设备策略和 runtime；落盘后移除旧 `/Applications/OpenSurge Menu Bar.app`。

## 卸载与登录项

卸载脚本安装在固定系统目录，必须 root-owned 且父目录不可由普通用户写入。它只在网关明确 stopped 后删除，不替用户停止网关；recovery 或宿主原有 forwarding 不单独阻断卸载。保留数据与彻底删除是两个固定范围，详情由原生确认展示，脚本成功后等待调用宿主自行退出。

宿主登录项清理、取消/失败恢复和 fresh-state 复核见 [桌面宿主契约](decisions/desktop-host.md)。同一 bundle ID、构建或 fixture 不能证明升级后的真实登录连续性；缺少当前 bundle 记录时提供显式注册，保留系统状态与错误，不重置 background-task 数据库。

## 旧宿主

`apps/menubar/`、旧构建和检查脚本保留供历史版本维护，已退出默认 GUI/发布链路。
显式 `make menubar-build` 输出到 `bin/legacy/OpenSurge.app`，不会覆盖 Wails 产物。
