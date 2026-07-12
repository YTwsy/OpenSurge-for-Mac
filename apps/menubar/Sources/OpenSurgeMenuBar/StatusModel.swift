import AppKit
import Foundation
import ServiceManagement

@MainActor
final class StatusModel: ObservableObject {
    @Published private(set) var status: MenuBarStatus?
    @Published private(set) var error: String?
    @Published private(set) var isRefreshing = false
    @Published var openAtLogin = false

    private let client: ControlAPIClient
    private var timer: Timer?
    private var rapidPolling = false
    private var failureCount = 0

    init(client: ControlAPIClient = ControlAPIClient()) {
        self.client = client
        self.openAtLogin = SMAppService.mainApp.status == .enabled
    }

    var indicator: IndicatorState { status?.indicator ?? .unreachable }

    func startPolling(rapid: Bool = false) {
        timer?.invalidate()
        rapidPolling = rapid
        Task { await refresh() }
    }

    func stopRapidPolling() { startPolling(rapid: false) }

    func refresh() async {
        guard !isRefreshing else { return }
        timer?.invalidate()
        isRefreshing = true
        defer { isRefreshing = false; scheduleNextRefresh() }
        do {
            status = try await client.status()
            error = nil
            failureCount = 0
        } catch ControlAPIError.descriptorUnavailable {
            await ControlServiceLauncher.wake()
            try? await Task.sleep(for: .milliseconds(350))
            do {
                status = try await client.status()
                error = nil
                failureCount = 0
            } catch {
                status = nil
                self.error = error.localizedDescription
                failureCount += 1
            }
        } catch {
            status = nil
            self.error = error.localizedDescription
            failureCount += 1
        }
    }

    private func scheduleNextRefresh() {
        let base = rapidPolling ? 2.0 : 15.0
        let multiplier = pow(2.0, Double(min(failureCount, 4)))
        let interval = min(base * multiplier, 60.0)
        timer = Timer.scheduledTimer(withTimeInterval: interval, repeats: false) { [weak self] _ in
            Task { @MainActor in await self?.refresh() }
        }
    }

    func openWebGUI(path: String = "dashboard") async {
        do {
			NSWorkspace.shared.open(try await client.bootstrapURL(path: path))
        } catch {
            self.error = error.localizedDescription
        }
    }

    func copyDiagnostics() {
        let text = status?.diagnosticSummary ?? "OpenSurge Control API: \(error ?? "unreachable")"
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(text, forType: .string)
    }
}
