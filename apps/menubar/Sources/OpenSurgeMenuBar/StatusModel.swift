import AppKit
import Foundation

@MainActor
final class StatusModel: ObservableObject {
    @Published private(set) var status: MenuBarStatus?
    @Published private(set) var error: String?
    @Published private(set) var isRefreshing = false
    @Published var openAtLogin = false

    private let client: ControlAPIClient
    private var timer: Timer?

    init(client: ControlAPIClient = ControlAPIClient()) {
        self.client = client
    }

    var indicator: IndicatorState { status?.indicator ?? .unreachable }

    func startPolling(rapid: Bool = false) {
        timer?.invalidate()
        timer = Timer.scheduledTimer(withTimeInterval: rapid ? 2 : 15, repeats: true) { [weak self] _ in
            Task { @MainActor in await self?.refresh() }
        }
        Task { await refresh() }
    }

    func stopRapidPolling() { startPolling(rapid: false) }

    func refresh() async {
        guard !isRefreshing else { return }
        isRefreshing = true
        defer { isRefreshing = false }
        do {
            status = try await client.status()
            error = nil
        } catch ControlAPIError.descriptorUnavailable {
            await ControlServiceLauncher.wake()
            try? await Task.sleep(for: .milliseconds(350))
            do {
                status = try await client.status()
                error = nil
            } catch {
                status = nil
                self.error = error.localizedDescription
            }
        } catch {
            status = nil
            self.error = error.localizedDescription
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
