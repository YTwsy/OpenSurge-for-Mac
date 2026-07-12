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
                if status.recoveryRequired {
                    recoveryCard(stage: status.recoveryStage ?? "required")
                } else {
                    statusGrid(status)
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

            if model.status?.recoveryRequired == true {
                Button { Task { await model.openWebGUI(path: "recovery") } } label: {
                    Label("继续恢复", systemImage: "wrench.and.screwdriver")
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
            row("Topology", status.topology)
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
            Text(stage).font(.caption).foregroundStyle(.secondary)
            Text("恢复路由器 DHCP 后，再让 Mac 和客户端回到自动获取。")
                .font(.caption).foregroundStyle(.secondary)
        }
        .padding(11).background(.orange.opacity(0.12), in: RoundedRectangle(cornerRadius: 10))
    }
}
