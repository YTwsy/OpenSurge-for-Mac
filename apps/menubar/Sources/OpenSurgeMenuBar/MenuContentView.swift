import ServiceManagement
import SwiftUI

struct MenuContentView: View {
    @ObservedObject var model: StatusModel

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack(spacing: 10) {
                Image(systemName: model.indicator.systemImage)
                    .font(.title2)
                    .foregroundStyle(model.indicator == .recovery ? .orange : .green)
                VStack(alignment: .leading, spacing: 2) {
                    Text("OpenSurge for Mac").font(.headline)
                    Text(model.indicator.accessibilityLabel).font(.caption).foregroundStyle(.secondary)
                }
                Spacer()
                if model.isRefreshing { ProgressView().controlSize(.small) }
            }

            if let status = model.status {
                if status.recoveryNeedsAttention {
                    recoveryCard(stage: status.recoveryStage ?? "required")
                } else {
                    statusGrid(status)
                }
                if status.takeoverActive {
                    Label(status.recoveryStage == "client_validated" ? "同一 LAN DHCP 接管已验收" : "同一 LAN DHCP 接管运行中，等待客户端验收", systemImage: "checkmark.shield")
                        .font(.caption).foregroundStyle(.green)
                }
                if status.recoverySnapshotPrepared {
                    Label("恢复资料已准备；尚未改动网络", systemImage: "doc.text")
                        .font(.caption).foregroundStyle(.secondary)
                }
                if status.drift {
                    Label("配置已修改，需要重启网关", systemImage: "arrow.triangle.2.circlepath")
                        .font(.caption).foregroundStyle(.orange)
                }
            } else {
                Label(model.error ?? "Control API 不可达", systemImage: "network.slash")
                    .font(.callout).foregroundStyle(.secondary)
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
                Label("打开 OpenSurge", systemImage: "arrow.up.forward.app")
                    .frame(maxWidth: .infinity)
            }.buttonStyle(.borderedProminent)

            if let status = model.status, status.recoveryRequired {
                Button { Task { await model.openWebGUI(path: "network") } } label: {
                    Label(status.recoveryNeedsAttention ? "继续恢复" : status.takeoverActive ? "查看接管状态" : "在网络设置中继续", systemImage: "wrench.and.screwdriver")
                        .frame(maxWidth: .infinity)
                }.buttonStyle(.bordered)
            }

            HStack {
                Button("复制诊断摘要") { model.copyDiagnostics() }
                Spacer()
                Button { Task { await model.refresh() } } label: { Image(systemName: "arrow.clockwise") }
                    .help("刷新")
            }

            Toggle("登录时显示", isOn: Binding(
                get: { model.openAtLogin },
                set: { value in
                    model.openAtLogin = value
                    try? value ? SMAppService.mainApp.register() : SMAppService.mainApp.unregister()
                }
            )).font(.caption)

            Button("退出菜单栏 App") { NSApplication.shared.terminate(nil) }
                .font(.caption).foregroundStyle(.secondary)
        }
        .padding(16)
        .frame(width: 330)
        .onAppear { model.startPolling(rapid: true) }
        .onDisappear { model.stopRapidPolling() }
    }

    private func statusGrid(_ status: MenuBarStatus) -> some View {
        Grid(alignment: .leading, horizontalSpacing: 14, verticalSpacing: 7) {
            row("Gateway", status.gateway.capitalized)
            row("Topology", status.topologyLabel)
            row("LAN IP", status.lanIp)
            row("Clients", String(status.clientCount))
            row("DHCP / DNS", status.dhcp)
            row("mihomo", status.mihomo)
            row("PF", status.pfAnchor)
            row("Forwarding", status.forwarding)
        }.font(.caption)
    }

    private func row(_ label: String, _ value: String) -> some View {
        GridRow { Text(label).foregroundStyle(.secondary); Text(value).lineLimit(1) }
    }

    private func recoveryCard(stage: String) -> some View {
        VStack(alignment: .leading, spacing: 5) {
            Label("网络恢复尚未完成", systemImage: "exclamationmark.triangle.fill").font(.subheadline).bold()
            Text(recoveryStageLabel(stage)).font(.caption).foregroundStyle(.secondary)
            Text("网络已开始变更。完成状态机并验证路由器 DHCP 恢复前，不要把 Mac 切回自动获取。")
                .font(.caption).foregroundStyle(.secondary)
        }
        .padding(11).background(.orange.opacity(0.12), in: RoundedRectangle(cornerRadius: 10))
    }
}

private func recoveryStageLabel(_ stage: String) -> String {
    switch stage {
    case "mac_static": "Mac 已使用固定 IPv4"
    case "router_dhcp_disabled_confirmed": "路由器 DHCP 已关闭"
    case "gateway_active": "OpenSurge 已接管"
    case "client_validated": "客户端 DHCP、DNS 与 TUN 已验收"
    case "gateway_stopped_waiting_router_dhcp": "已停止，等待恢复路由器 DHCP"
    case "router_dhcp_restored": "路由器 DHCP 已恢复"
    default: stage
    }
}
