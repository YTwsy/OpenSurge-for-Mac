import AppKit
import Foundation
import ServiceManagement

@MainActor
final class StatusModel: ObservableObject {
    @Published private(set) var status: MenuBarStatus?
    @Published private(set) var error: String?
    @Published private(set) var isRefreshing = false
    @Published private(set) var serviceNeedsReconnect = false
    @Published private(set) var isChangingServices = false
    @Published private(set) var isUninstalling = false
    @Published private(set) var isCheckingForUpdate = false
    @Published private(set) var availableUpdate: AvailableUpdate?
    @Published private(set) var updateCheckMessage: String?
    @Published var openAtLogin = false

    private let client: ControlAPIClient
    private let urlLauncher: WebGUIURLLauncher
    private let updateChecker: UpdateChecker
    let currentVersion: String
    private var timer: Timer?
    private var rapidPolling = false
    private var failureCount = 0
    private var isQuitting = false
    private var lastAutomaticUpdateCheck: Date?

    init(
        client: ControlAPIClient = ControlAPIClient(),
        urlLauncher: WebGUIURLLauncher = WebGUIURLLauncher(),
        updateChecker: UpdateChecker = UpdateChecker(),
        currentVersion: String = Bundle.main.object(
            forInfoDictionaryKey: "CFBundleShortVersionString"
        ) as? String ?? "未知"
    ) {
        self.client = client
        self.urlLauncher = urlLauncher
        self.updateChecker = updateChecker
        self.currentVersion = currentVersion
        self.openAtLogin = SMAppService.mainApp.status == .enabled
    }

    var indicator: IndicatorState { menuBarIndicator(status: status, hasError: error != nil) }
    var canQuitOpenSurge: Bool { status?.canQuitOpenSurge == true && !isChangingServices }
    var canUninstall: Bool { status?.canUninstall == true && !isChangingServices }

    func startPolling(rapid: Bool = false) {
        guard !isQuitting else { return }
        timer?.invalidate()
        rapidPolling = rapid
        Task { await refresh() }
        checkForUpdatesAutomaticallyIfNeeded()
    }

    func stopRapidPolling() {
        guard !isQuitting else { return }
        startPolling(rapid: false)
    }

    func refresh() async {
        guard !isQuitting, !isRefreshing else { return }
        timer?.invalidate()
        isRefreshing = true
        defer { isRefreshing = false; scheduleNextRefresh() }
        do {
            status = try await client.status()
            error = nil
            serviceNeedsReconnect = false
            failureCount = 0
        } catch let controlError as ControlAPIError where controlError.serviceUnavailable {
            guard !isQuitting else { return }
            await ControlServiceLauncher.wake()
            guard !isQuitting else { return }
            try? await Task.sleep(for: .milliseconds(350))
            guard !isQuitting else { return }
            do {
                status = try await client.status()
                error = nil
                serviceNeedsReconnect = false
                failureCount = 0
            } catch {
                recordFailure(error)
            }
        } catch {
            recordFailure(error)
        }
    }

    func reconnectService() async {
        guard !isQuitting, !isRefreshing else { return }
        timer?.invalidate()
        isRefreshing = true
        error = nil
        serviceNeedsReconnect = false
        await ControlServiceLauncher.wake(restart: true)
        try? await Task.sleep(for: .milliseconds(350))
        isRefreshing = false
        await refresh()
    }

    func quitMenuBarApp() -> Never {
        isQuitting = true
        timer?.invalidate()
        ControlServiceLauncher.terminateMenuBarApp()
    }

    func quitOpenSurge() {
        guard canQuitOpenSurge else {
            error = openSurgeQuitWarning(for: status)
            return
        }
        timer?.invalidate()
        isQuitting = true
        isChangingServices = true
        error = nil
        Task {
            do {
                try await ControlServiceLauncher.stopControlService()
                ControlServiceLauncher.terminateMenuBarApp()
            } catch {
                isQuitting = false
                self.error = error.localizedDescription
                isChangingServices = false
                scheduleNextRefresh()
            }
        }
    }

    func uninstall(_ mode: UninstallMode) {
        guard canUninstall else {
            error = uninstallWarning(for: status)
            return
        }
        timer?.invalidate()
        isQuitting = true
        isChangingServices = true
        isUninstalling = true
        error = nil
        let restoreLoginItemOnFailure = openAtLogin
        if restoreLoginItemOnFailure {
            try? SMAppService.mainApp.unregister()
            openAtLogin = false
        }
        Task {
            do {
                try await OpenSurgeUninstaller.run(mode: mode)
                ControlServiceLauncher.terminateMenuBarApp()
            } catch OpenSurgeUninstallError.authorizationCancelled {
                restoreAfterUninstallFailure(
                    error: nil,
                    restoreLoginItem: restoreLoginItemOnFailure
                )
            } catch {
                restoreAfterUninstallFailure(
                    error: error.localizedDescription,
                    restoreLoginItem: restoreLoginItemOnFailure
                )
            }
        }
    }

    private func restoreAfterUninstallFailure(error: String?, restoreLoginItem: Bool) {
        if restoreLoginItem {
            try? SMAppService.mainApp.register()
            openAtLogin = true
        }
        isQuitting = false
        isChangingServices = false
        isUninstalling = false
        self.error = error
        scheduleNextRefresh()
    }

    private func recordFailure(_ error: Error) {
        guard !isQuitting else { return }
        status = nil
        self.error = error.localizedDescription
        serviceNeedsReconnect = (error as? ControlAPIError)?.serviceUnavailable ?? false
        failureCount += 1
    }

    private func scheduleNextRefresh() {
        guard !isQuitting else { return }
        let base = rapidPolling ? 2.0 : 15.0
        let multiplier = pow(2.0, Double(min(failureCount, 4)))
        let interval = min(base * multiplier, 60.0)
        timer = Timer.scheduledTimer(withTimeInterval: interval, repeats: false) { [weak self] _ in
            guard let model = self else { return }
            Task { @MainActor in await model.refresh() }
        }
    }

    func openWebGUI(path: String = "dashboard") async {
        do {
            let url = try await client.bootstrapURL(path: path)
            try urlLauncher.open(url)
        } catch {
            self.error = error.localizedDescription
        }
    }

    func copyDiagnostics() {
        let text = status?.diagnosticSummary ?? "OpenSurge Control API: \(error ?? "unreachable")"
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(text, forType: .string)
    }

    func checkForUpdates(manual: Bool = true) async {
        guard !isQuitting, !isCheckingForUpdate else { return }
        isCheckingForUpdate = true
        if manual { updateCheckMessage = nil }
        defer { isCheckingForUpdate = false }

        do {
            availableUpdate = try await updateChecker.check(currentVersion: currentVersion)
            updateCheckMessage = availableUpdate == nil ? "当前已是最新稳定版" : nil
        } catch {
            updateCheckMessage = manual
                ? "检查更新失败：\(error.localizedDescription)"
                : "自动检查更新失败，可手动重试"
        }
    }

    func openUpdateDownloadPage() {
        guard let availableUpdate else { return }
        do {
            try urlLauncher.open(availableUpdate.releasePage)
        } catch {
            updateCheckMessage = "无法打开下载页，请前往 OpenSurge GitHub Releases"
        }
    }

    private func checkForUpdatesAutomaticallyIfNeeded() {
        let now = Date()
        if let lastAutomaticUpdateCheck,
           now.timeIntervalSince(lastAutomaticUpdateCheck) < 24 * 60 * 60 {
            return
        }
        lastAutomaticUpdateCheck = now
        Task { await checkForUpdates(manual: false) }
    }
}
