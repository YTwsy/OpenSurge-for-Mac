import AppKit
import Foundation
import Darwin

enum CheckFailure: Error, CustomStringConvertible {
    case failed(String)
    var description: String { if case .failed(let message) = self { return message }; return "check failed" }
}

func require(_ condition: @autoclosure () -> Bool, _ message: String) throws {
    if !condition() { throw CheckFailure.failed(message) }
}

private final class CheckURLProtocol: URLProtocol {
    nonisolated(unsafe) static var handler: ((URLRequest) throws -> (HTTPURLResponse, Data))?
    nonisolated(unsafe) static var lastFailure: String?
    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
    override func startLoading() {
        do {
            guard let handler = Self.handler else { throw CheckFailure.failed("missing URLProtocol handler") }
            let (response, data) = try handler(request)
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch { Self.lastFailure = String(describing: error); client?.urlProtocol(self, didFailWithError: error) }
    }
    override func stopLoading() {}
}

@main
struct MenuBarChecks {
    static func main() async {
        do { try await run() }
        catch { fputs("OpenSurge menu bar checks failed: \(error)\n", stderr); exit(1) }
    }

    static func run() async throws {
        let directory = FileManager.default.temporaryDirectory.appending(path: "opensurge-menubar-check-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: directory) }
        try Data(#"{"schema_version":1,"url":"http://127.0.0.1:61767"}"#.utf8).write(to: directory.appending(path: "control-endpoint.json"))
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [CheckURLProtocol.self]
        let client = ControlAPIClient(session: URLSession(configuration: configuration), applicationSupport: directory, tokenOverride: "test-token")

        CheckURLProtocol.handler = { request in
            try require(request.value(forHTTPHeaderField: "Authorization") == "Bearer test-token", "status bearer token missing")
            try require(request.url?.path == "/api/v1/menubar", "status path mismatch")
            let body = #"{"schema_version":1,"revision":"r1","gateway":"running","topology":"same_wifi_dhcp","lan_ip":"192.168.1.20","dhcp":"running","mihomo":"running","pf_anchor":"loaded","forwarding":"enabled","client_count":2,"drift":false,"doctor_healthy":true,"recovery_required":false,"warnings":[]}"#
            return (HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!, Data(body.utf8))
        }
        let status: MenuBarStatus
        do { status = try await client.status() }
        catch { throw CheckFailure.failed("status request failed: \(CheckURLProtocol.lastFailure ?? String(describing: error))") }
        try require(status.indicator == .running, "running indicator mismatch")
        try require(menuBarIndicator(status: nil, hasError: false) == .connecting, "initial menu bar state must use the brand icon")
        try require(menuBarIndicator(status: nil, hasError: true) == .unreachable, "confirmed Control Service failure must remain distinguishable")
        try require(IndicatorState.connecting.usesBrandMenuBarIcon && IndicatorState.connecting.menuBarIconOpacity == 0.75, "connecting indicator must use the brand icon")
        try require(IndicatorState.unreachable.usesBrandMenuBarIcon && IndicatorState.unreachable.menuBarIconOpacity == 0.35, "initial Control Service delay must not restore the legacy-looking icon")
        try require(status.topologyLabel == "局域网 DHCP 接管", "DHCP takeover topology label mismatch")
        try require(status.gatewayServicesActive && menuBarQuitWarning(for: status).contains("都不会停止"), "active gateway quit warning mismatch")
        try require(status.diagnosticSummary.contains("PF: loaded"), "diagnostic summary omitted PF")

        try Data("file-token".utf8).write(to: directory.appending(path: "control-token"))
        let fileTokenClient = ControlAPIClient(session: URLSession(configuration: configuration), applicationSupport: directory)
        CheckURLProtocol.handler = { request in
            try require(request.value(forHTTPHeaderField: "Authorization") == "Bearer file-token", "application support token was not used")
            let body = #"{"schema_version":1,"revision":"r1","gateway":"stopped","topology":"isolated_lan","lan_ip":"192.168.50.1","dhcp":"stopped","mihomo":"stopped","pf_anchor":"unloaded","forwarding":"disabled","client_count":0,"drift":false,"doctor_healthy":true,"recovery_required":false,"warnings":[]}"#
            return (HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!, Data(body.utf8))
        }
        _ = try await fileTokenClient.status()

        CheckURLProtocol.handler = { _ in throw URLError(.cannotConnectToHost) }
        do {
            _ = try await client.status()
            throw CheckFailure.failed("transport failure did not fail")
        } catch let error as ControlAPIError {
            try require(error.serviceUnavailable, "transport failure did not trigger Control Service recovery")
        }

        try FileManager.default.removeItem(at: directory.appending(path: "control-token"))
        do {
            _ = try await fileTokenClient.status()
            throw CheckFailure.failed("missing local token did not fail")
        } catch let error as ControlAPIError {
            try require(error.serviceUnavailable, "missing local token was not classified as a service availability failure")
            try require(error.localizedDescription == "OpenSurge 后台服务尚未准备好", "missing local token exposed a technical error")
        }

        CheckURLProtocol.handler = { request in
            try require(request.url?.query == nil, "long-lived token leaked into request URL")
            try require(request.value(forHTTPHeaderField: "Authorization") == "Bearer test-token", "bootstrap bearer token missing")
            try require(String(decoding: requestBody(request), as: UTF8.self) == #"{"path":"network"}"#, "bootstrap deep-link body mismatch")
            let body = #"{"schema_version":1,"url":"http://127.0.0.1:61767/bootstrap?code=one-time","expires_at":"2026-07-12T00:00:00.123456789Z"}"#
            return (HTTPURLResponse(url: request.url!, statusCode: 201, httpVersion: nil, headerFields: nil)!, Data(body.utf8))
        }
        let bootstrap: URL
        do { bootstrap = try await client.bootstrapURL(path: "network") }
        catch { throw CheckFailure.failed("bootstrap request failed: \(CheckURLProtocol.lastFailure ?? String(describing: error))") }
        try require(bootstrap.query == "code=one-time" && !bootstrap.absoluteString.contains("test-token"), "bootstrap URL leaked long-lived token")

        let active = MenuBarStatus(schemaVersion: 1, revision: "r", gateway: "running", topology: "same_wifi_dhcp", lanIp: "192.168.1.20", dhcp: "running", mihomo: "running", pfAnchor: "loaded", forwarding: "enabled", clientCount: 2, drift: false, doctorHealthy: true, recoveryRequired: true, recoveryStage: "gateway_active", warnings: [], errorCode: nil)
        try require(active.takeoverActive && !active.recoveryNeedsAttention && active.indicator == .running, "active takeover must use the running indicator")
        let recovery = MenuBarStatus(schemaVersion: 1, revision: "r", gateway: "stopped", topology: "same_wifi_dhcp", lanIp: "192.168.1.20", dhcp: "stopped", mihomo: "stopped", pfAnchor: "unloaded", forwarding: "disabled", clientCount: 2, drift: true, doctorHealthy: false, recoveryRequired: true, recoveryStage: "gateway_stopped_waiting_router_dhcp", warnings: [], errorCode: nil)
        try require(recovery.recoveryNeedsAttention && recovery.indicator == .recovery && recovery.indicator.systemImage == "exclamationmark.triangle.fill", "post-stop recovery must have highest priority")
        let prepared = MenuBarStatus(schemaVersion: 1, revision: "r", gateway: "stopped", topology: "same_wifi_dhcp", lanIp: "192.168.1.20", dhcp: "stopped", mihomo: "stopped", pfAnchor: "unloaded", forwarding: "disabled", clientCount: 0, drift: false, doctorHealthy: true, recoveryRequired: true, recoveryStage: "prepared", warnings: [], errorCode: nil)
        try require(prepared.recoverySnapshotPrepared && !prepared.recoveryNeedsAttention && prepared.indicator == .stopped, "prepared recovery must not present as a network recovery")
        let stopped = MenuBarStatus(schemaVersion: 1, revision: "r", gateway: "stopped", topology: "same_wifi_dhcp", lanIp: "192.168.1.20", dhcp: "stopped", mihomo: "stopped", pfAnchor: "unloaded", forwarding: "disabled", clientCount: 0, drift: true, doctorHealthy: false, recoveryRequired: false, recoveryStage: nil, warnings: [], errorCode: nil)
        try require(stopped.indicator == .stopped && stopped.indicator.accessibilityLabel == "OpenSurge 网关已停止", "stopped gateway must not be presented as a runtime failure")
        try require(stopped.canQuitOpenSurge && openSurgeQuitWarning(for: stopped).contains("root Helper 仍保持空闲加载"), "stopped gateway must allow the explicit OpenSurge quit path")
        try require(!active.canQuitOpenSurge && !recovery.canQuitOpenSurge, "active or recovery state must block the OpenSurge quit path")

        let (useDefaultReopen, presentationCount) = await MainActor.run {
            let panelPresenter = CheckMenuBarPresenter()
            let appDelegate = OpenSurgeAppDelegate(presenter: panelPresenter)
            let useDefaultReopen = appDelegate.applicationShouldHandleReopen(NSApplication.shared, hasVisibleWindows: false)
            return (useDefaultReopen, panelPresenter.presentationCount)
        }
        try require(!useDefaultReopen && presentationCount == 1, "opening OpenSurge again must show the menu bar panel")

        let launchPresentationCount = await presentationCountAfterLaunch()
        try require(launchPresentationCount == 1, "launching OpenSurge must show the menu bar panel")

        var fallbackOpened = false
        let launcher = WebGUIURLLauncher(
            workspaceOpen: { _ in false },
            commandOpen: { _ in fallbackOpened = true }
        )
        try launcher.open(URL(string: "http://127.0.0.1:61767/bootstrap?code=test")!)
        try require(fallbackOpened, "workspace URL failure did not use the open command fallback")

        let failingLauncher = WebGUIURLLauncher(
            workspaceOpen: { _ in false },
            commandOpen: { _ in throw CheckFailure.failed("simulated browser failure") }
        )
        do {
            try failingLauncher.open(URL(string: "http://127.0.0.1:61767/bootstrap?code=test")!)
            throw CheckFailure.failed("browser failure was silently ignored")
        } catch WebGUIURLLaunchError.browserUnavailable {
            // Expected: the caller can surface this without leaking the bootstrap URL.
        }

        print("OpenSurge menu bar checks passed")
    }
}

@MainActor
private func presentationCountAfterLaunch() async -> Int {
    let panelPresenter = CheckMenuBarPresenter()
    let appDelegate = OpenSurgeAppDelegate(presenter: panelPresenter)
    appDelegate.applicationDidFinishLaunching(
        Notification(name: NSApplication.didFinishLaunchingNotification)
    )
    await withCheckedContinuation { (continuation: CheckedContinuation<Void, Never>) in
        DispatchQueue.main.async {
            continuation.resume()
        }
    }
    withExtendedLifetime(appDelegate) {}
    return panelPresenter.presentationCount
}

@MainActor
private final class CheckMenuBarPresenter: MenuBarPresenting {
    var presentationCount = 0
    func showPanel() { presentationCount += 1 }
}

private func requestBody(_ request: URLRequest) -> Data {
    if let body = request.httpBody { return body }
    guard let stream = request.httpBodyStream else { return Data() }
    stream.open(); defer { stream.close() }
    var result = Data(), buffer = [UInt8](repeating: 0, count: 4096)
    while stream.hasBytesAvailable {
        let count = stream.read(&buffer, maxLength: buffer.count)
        if count <= 0 { break }
        result.append(buffer, count: count)
    }
    return result
}
