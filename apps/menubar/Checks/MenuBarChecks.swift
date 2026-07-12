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
        try require(status.diagnosticSummary.contains("PF: loaded"), "diagnostic summary omitted PF")

        CheckURLProtocol.handler = { request in
            try require(request.url?.query == nil, "long-lived token leaked into request URL")
            try require(request.value(forHTTPHeaderField: "Authorization") == "Bearer test-token", "bootstrap bearer token missing")
            try require(String(decoding: requestBody(request), as: UTF8.self) == #"{"path":"recovery"}"#, "bootstrap deep-link body mismatch")
            let body = #"{"schema_version":1,"url":"http://127.0.0.1:61767/bootstrap?code=one-time","expires_at":"2026-07-12T00:00:00.123456789Z"}"#
            return (HTTPURLResponse(url: request.url!, statusCode: 201, httpVersion: nil, headerFields: nil)!, Data(body.utf8))
        }
        let bootstrap: URL
        do { bootstrap = try await client.bootstrapURL(path: "recovery") }
        catch { throw CheckFailure.failed("bootstrap request failed: \(CheckURLProtocol.lastFailure ?? String(describing: error))") }
        try require(bootstrap.query == "code=one-time" && !bootstrap.absoluteString.contains("test-token"), "bootstrap URL leaked long-lived token")

        let recovery = MenuBarStatus(schemaVersion: 1, revision: "r", gateway: "running", topology: "same_wifi_dhcp", lanIp: "192.168.1.20", dhcp: "running", mihomo: "running", pfAnchor: "loaded", forwarding: "enabled", clientCount: 2, drift: true, doctorHealthy: false, recoveryRequired: true, recoveryStage: "gateway_active", warnings: [], errorCode: nil)
        try require(recovery.indicator == .recovery && recovery.indicator.systemImage == "exclamationmark.triangle.fill", "recovery indicator must have highest priority")

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
