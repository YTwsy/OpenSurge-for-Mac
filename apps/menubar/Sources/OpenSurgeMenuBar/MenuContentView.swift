import AppKit
import ServiceManagement
import SwiftUI

struct MenuContentView: View {
    @ObservedObject var model: StatusModel

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack(spacing: 10) {
                OpenSurgeAppIconView()
                    .accessibilityHidden(true)
                VStack(alignment: .leading, spacing: 2) {
                    Text("OpenSurge for Mac").font(.headline)
                    Text(L10n.text(model.indicator.accessibilityLabel)).font(.caption).foregroundStyle(.secondary)
                }
                Spacer()
            }

            if let status = model.status {
                if status.recoveryNeedsAttention {
                    recoveryCard(stage: status.recoveryStage ?? "required")
                } else {
                    statusGrid(status)
                }
                if status.takeoverActive {
                    Label(takeoverStatusLabel(status.recoveryStage), systemImage: "checkmark.shield")
                        .font(.caption).foregroundStyle(.green)
                }
                if status.recoverySnapshotPrepared {
                    Label(L10n.text("恢复资料已准备；尚未改动网络"), systemImage: "doc.text")
                        .font(.caption).foregroundStyle(.secondary)
                }
                if status.drift {
                    Label(L10n.text("配置已修改，需要重启网关"), systemImage: "arrow.triangle.2.circlepath")
                        .font(.caption).foregroundStyle(.orange)
                }
            } else {
                VStack(alignment: .leading, spacing: 8) {
                    Label(
                        model.isRefreshing && model.error == nil
                            ? L10n.text("正在连接 OpenSurge 后台服务…")
                            : model.error ?? L10n.text("OpenSurge 后台服务尚未准备好"),
                        systemImage: model.isRefreshing && model.error == nil ? "network" : "network.slash"
                    )
                        .font(.callout).foregroundStyle(.secondary)
                    if model.serviceNeedsReconnect {
                        Button { Task { await model.reconnectService() } } label: {
                            Label(L10n.text("重新连接"), systemImage: "arrow.clockwise")
                        }
                        .buttonStyle(.bordered)
                        .disabled(model.isRefreshing)
                    }
                }
                .padding(.vertical, 8)
            }

            if let error = model.error, model.status != nil {
                Label(error, systemImage: "exclamationmark.triangle")
                    .font(.caption)
                    .foregroundStyle(.red)
                    .fixedSize(horizontal: false, vertical: true)
            }

            Divider()
            Button { Task { await model.openWebGUI() } } label: {
                Label(L10n.text("打开 OpenSurge 面板"), systemImage: "arrow.up.forward.app")
                    .frame(maxWidth: .infinity)
            }.buttonStyle(.borderedProminent)

            if let status = model.status, status.recoveryRequired {
                Button { Task { await model.openWebGUI(path: "network") } } label: {
                    Label(L10n.text(status.recoveryNeedsAttention ? "继续恢复" : status.takeoverActive ? "查看接管状态" : "在网络设置中继续"), systemImage: "wrench.and.screwdriver")
                        .frame(maxWidth: .infinity)
                }.buttonStyle(.bordered)
            }

            HStack {
                Button(L10n.text("复制诊断摘要")) { model.copyDiagnostics() }
                Spacer()
                Button { Task { await model.refresh() } } label: { Image(systemName: "arrow.clockwise") }
                    .help(L10n.text("刷新"))
            }

            Toggle(L10n.text("登录时显示"), isOn: Binding(
                get: { model.openAtLogin },
                set: { value in
                    model.openAtLogin = value
                    try? value ? SMAppService.mainApp.register() : SMAppService.mainApp.unregister()
                }
            )).font(.caption)

            Toggle(L10n.text("合盖保持运行"), isOn: Binding(
                get: { model.status?.sleepPrevention?.active == true },
                set: { value in Task { await model.setSleepPrevention(value) } }
            ))
                .font(.caption)
                .disabled(model.status == nil || model.isChangingSleepPrevention)
                .help(L10n.text("临时禁用系统睡眠，包括合盖睡眠；默认关闭，退出 OpenSurge 或重启 Mac 后自动释放。"))

            if model.status?.sleepPrevention?.active == true {
                Label(L10n.text("合盖后仍会运行，请注意耗电与散热，不要放入不通风的包内。"), systemImage: "exclamationmark.triangle")
                    .font(.caption2)
                    .foregroundStyle(.orange)
                    .fixedSize(horizontal: false, vertical: true)
            } else if let sleepError = model.status?.sleepPrevention?.error {
                Text(sleepError)
                    .font(.caption2)
                    .foregroundStyle(.red)
                    .fixedSize(horizontal: false, vertical: true)
            }

            Divider()
            VStack(alignment: .leading, spacing: 7) {
                HStack {
                    Text(L10n.format("版本 %@", releaseDisplayVersion(model.currentVersion)))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    Spacer()
                    Button(L10n.text(model.isCheckingForUpdate ? "正在检查…" : "检查更新")) {
                        Task { await model.checkForUpdates() }
                    }
                    .font(.caption)
                    .disabled(model.isCheckingForUpdate)
                }
                if let update = model.availableUpdate {
                    Label(L10n.format("发现稳定版 %@", update.version), systemImage: "arrow.down.circle.fill")
                        .font(.caption)
                        .foregroundStyle(.blue)
                    Button(L10n.format("打开 %@ 下载页", update.version)) {
                        model.openUpdateDownloadPage()
                    }
                    .buttonStyle(.bordered)
                    .frame(maxWidth: .infinity, alignment: .leading)
                } else if let message = model.updateCheckMessage {
                    Text(message)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }

            Divider()
            Button(L10n.text("退出 OpenSurge…")) { confirmQuit(.openSurge) }
                .font(.caption)
                .disabled(!model.canQuitOpenSurge)
                .help(fullQuitHelp)
            Button(L10n.text("只退出菜单栏 App…")) { confirmQuit(.menuBarOnly) }
                .font(.caption).foregroundStyle(.secondary)
                .disabled(model.isChangingServices)
            Divider()
            Button(L10n.text(model.isUninstalling ? "正在卸载…" : "卸载 OpenSurge…")) {
                confirmUninstall()
            }
                .font(.caption)
                .foregroundStyle(.red)
                .disabled(!model.canUninstall)
                .help(uninstallHelp)
        }
        .padding(16)
        .frame(width: 330)
        .onAppear { model.startPolling(rapid: true) }
        .onDisappear { model.stopRapidPolling() }
    }

    private var fullQuitHelp: String {
        if model.status?.recoveryNeedsAttention == true { return L10n.text("请先完成网络恢复") }
        if model.status?.canQuitOpenSurge != true { return L10n.text("请先在网络设置中停止网关") }
        return L10n.text("停止用户级 Control Service，然后退出菜单栏 App；root Helper 保持空闲加载")
    }

    private var uninstallHelp: String {
        if model.status == nil { return L10n.text("请先重新连接后台服务") }
        if model.status?.canUninstall != true { return L10n.text("请先在网络设置中停止网关") }
        return L10n.text("移除 OpenSurge App、后台服务与 root Helper")
    }

    private func confirmQuit(_ confirmation: QuitConfirmation) {
        guard confirmation.present(for: model.status) else { return }
        switch confirmation {
        case .openSurge:
            model.quitOpenSurge()
        case .menuBarOnly:
            model.quitMenuBarApp()
        }
    }

    private func confirmUninstall() {
        guard let mode = UninstallConfirmation.present(for: model.status) else { return }
        model.uninstall(mode)
    }

    private func statusGrid(_ status: MenuBarStatus) -> some View {
        Grid(alignment: .leading, horizontalSpacing: 14, verticalSpacing: 7) {
            row(L10n.text("网关"), localizedRuntimeState(status.gateway))
            row(L10n.text("拓扑"), L10n.text(status.topologyLabel))
            row("LAN IP", status.lanIp)
            row(L10n.text("客户端"), String(status.clientCount))
            row("DHCP / DNS", localizedRuntimeState(status.dhcp))
            row("mihomo", localizedRuntimeState(status.mihomo))
            row("TUN", status.tunInterface.map { "\(localizedRuntimeState(status.tun ?? "unknown")) · \($0)" } ?? localizedRuntimeState(status.tun ?? "unknown"))
            row("PF", localizedRuntimeState(status.pfAnchor))
            row(L10n.text("IPv4 转发"), localizedRuntimeState(status.forwarding))
        }.font(.caption)
    }

    private func row(_ label: String, _ value: String) -> some View {
        GridRow { Text(label).foregroundStyle(.secondary); Text(value).lineLimit(1) }
    }

    private func recoveryCard(stage: String) -> some View {
        VStack(alignment: .leading, spacing: 5) {
            Label(L10n.text("网络恢复尚未完成"), systemImage: "exclamationmark.triangle.fill").font(.subheadline).bold()
            Text(recoveryStageLabel(stage)).font(.caption).foregroundStyle(.secondary)
            Text(L10n.text("网络已开始变更。完成状态机并验证路由器 DHCP 恢复前，不要把 Mac 切回自动获取。"))
                .font(.caption).foregroundStyle(.secondary)
        }
        .padding(11).background(.orange.opacity(0.12), in: RoundedRectangle(cornerRadius: 10))
    }
}

private func localizedRuntimeState(_ state: String) -> String {
    switch state.lowercased() {
    case "running": L10n.text("运行中")
    case "stopped": L10n.text("已停止")
    case "degraded": L10n.text("异常")
    case "disabled": L10n.text("已禁用")
    case "enabled": L10n.text("已启用")
    case "loaded": L10n.text("已加载")
    case "unloaded": L10n.text("未加载")
    case "ready": L10n.text("就绪")
    case "failed": L10n.text("失败")
    case "unknown": L10n.text("未知")
    default: state
    }
}

@MainActor
private enum UninstallConfirmation {
    static func present(for status: MenuBarStatus?) -> UninstallMode? {
        guard status?.canUninstall == true else { return nil }

        let alert = NSAlert()
        alert.alertStyle = .warning
        alert.messageText = L10n.text("卸载 OpenSurge？")
        alert.informativeText = L10n.text("OpenSurge App、用户级 Control Service 与 root Helper 都会被移除。\n\n“保留数据并卸载”会保留配置、订阅和策略数据，便于重新安装；“彻底卸载”还会删除凭据、运行记录和日志。当前系统的 IPv4 forwarding 状态不会被修改。")
        alert.addButton(withTitle: L10n.text("保留数据并卸载"))
        alert.addButton(withTitle: L10n.text("彻底卸载")).hasDestructiveAction = true
        alert.addButton(withTitle: L10n.text("取消"))

        switch alert.runModal() {
        case .alertFirstButtonReturn: return .keepData
        case .alertSecondButtonReturn: return .removeEverything
        default: return nil
        }
    }
}

@MainActor
private enum QuitConfirmation {
    case menuBarOnly
    case openSurge

    var title: String {
        L10n.text(self == .openSurge ? "退出 OpenSurge？" : "只退出菜单栏 App？")
    }

    var buttonTitle: String {
        L10n.text(self == .openSurge ? "退出 OpenSurge" : "仍然退出")
    }

    func message(for status: MenuBarStatus?) -> String {
        self == .openSurge
            ? L10n.text(openSurgeQuitWarning(for: status))
            : L10n.text(menuBarQuitWarning(for: status))
    }

    func present(for status: MenuBarStatus?) -> Bool {
        let alert = NSAlert()
        alert.alertStyle = .warning
        alert.messageText = title
        alert.informativeText = message(for: status)
        alert.addButton(withTitle: buttonTitle).hasDestructiveAction = true
        alert.addButton(withTitle: L10n.text("取消"))
        return alert.runModal() == .alertFirstButtonReturn
    }
}

private func recoveryStageLabel(_ stage: String) -> String {
    switch stage {
    case "mac_static": L10n.text("Mac 已使用固定 IPv4")
    case "router_dhcp_disabled_confirmed": L10n.text("路由器 DHCP 已关闭")
    case "gateway_active": L10n.text("OpenSurge 已接管")
    case "client_validated": L10n.text("客户端 DHCP、DNS 与 TUN 已验收")
    case "client_validation_skipped": L10n.text("客户端验收已跳过")
    case "gateway_stopped_waiting_router_dhcp": L10n.text("已停止，等待恢复路由器 DHCP")
    case "router_dhcp_restored": L10n.text("路由器 DHCP 已恢复")
    default: stage
    }
}

private func takeoverStatusLabel(_ stage: String?) -> String {
    switch stage {
    case "client_validated": L10n.text("局域网 DHCP 接管已验收")
    case "client_validation_skipped": L10n.text("局域网 DHCP 接管运行中，客户端验收已跳过")
    default: L10n.text("局域网 DHCP 接管运行中，等待客户端验收")
    }
}
