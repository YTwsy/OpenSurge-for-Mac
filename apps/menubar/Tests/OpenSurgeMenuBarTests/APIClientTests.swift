import XCTest
@testable import OpenSurgeMenuBar

final class APIClientTests: XCTestCase {
    override func tearDown() {
        MockURLProtocol.handler = nil
        super.tearDown()
    }

    func testStatusUsesBearerTokenAndDecodesMinimalDTO() async throws {
        let client = try makeClient()
        MockURLProtocol.handler = { request in
            XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "Bearer test-token")
            XCTAssertEqual(request.url?.path, "/api/v1/menubar")
            let body = #"{"schema_version":1,"revision":"r1","gateway":"stopped","topology":"same_wifi_dhcp","lan_ip":"192.168.1.20","dhcp":"stopped","mihomo":"stopped","pf_anchor":"unloaded","forwarding":"disabled","client_count":0,"drift":false,"doctor_healthy":true,"recovery_required":false,"warnings":[]}"#
            return (HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!, Data(body.utf8))
        }
        let status = try await client.status()
        XCTAssertEqual(status.gateway, "stopped")
        XCTAssertEqual(status.indicator, .stopped)
    }

    func testBootstrapSendsRequestedDeepLinkWithoutPuttingTokenInURL() async throws {
        let client = try makeClient()
        MockURLProtocol.handler = { request in
            XCTAssertEqual(request.url?.query, nil)
            XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "Bearer test-token")
            let body = try XCTUnwrap(request.httpBody)
            XCTAssertEqual(String(decoding: body, as: UTF8.self), #"{"path":"recovery"}"#)
            let response = #"{"schema_version":1,"url":"http://127.0.0.1:61767/bootstrap?code=one-time","expires_at":"2026-07-12T00:00:00.123456789Z"}"#
            return (HTTPURLResponse(url: request.url!, statusCode: 201, httpVersion: nil, headerFields: nil)!, Data(response.utf8))
        }
        let url = try await client.bootstrapURL(path: "recovery")
        XCTAssertEqual(url.query, "code=one-time")
        XCTAssertFalse(url.absoluteString.contains("test-token"))
    }

    func testStatusReadsLocalTokenFromApplicationSupport() async throws {
        let client = try makeClient(tokenOverride: nil, fileToken: "file-token")
        MockURLProtocol.handler = { request in
            XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "Bearer file-token")
            let body = #"{"schema_version":1,"revision":"r1","gateway":"stopped","topology":"isolated_lan","lan_ip":"192.168.50.1","dhcp":"stopped","mihomo":"stopped","pf_anchor":"unloaded","forwarding":"disabled","client_count":0,"drift":false,"doctor_healthy":true,"recovery_required":false,"warnings":[]}"#
            return (HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!, Data(body.utf8))
        }

        _ = try await client.status()
    }

    func testMissingLocalTokenUsesFriendlyServiceUnavailableError() async throws {
        let client = try makeClient(tokenOverride: nil)
        do {
            _ = try await client.status()
            XCTFail("expected missing token to fail")
        } catch let error as ControlAPIError {
            XCTAssertTrue(error.serviceUnavailable)
            XCTAssertEqual(error.localizedDescription, "OpenSurge 后台服务尚未准备好")
        }
    }

    private func makeClient(tokenOverride: String? = "test-token", fileToken: String? = nil) throws -> ControlAPIClient {
        let directory = FileManager.default.temporaryDirectory.appending(path: UUID().uuidString)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let descriptor = #"{"schema_version":1,"url":"http://127.0.0.1:61767"}"#
        try Data(descriptor.utf8).write(to: directory.appending(path: "control-endpoint.json"))
        if let fileToken {
            try Data(fileToken.utf8).write(to: directory.appending(path: "control-token"))
        }
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [MockURLProtocol.self]
        return ControlAPIClient(session: URLSession(configuration: configuration), applicationSupport: directory, tokenOverride: tokenOverride)
    }
}

private final class MockURLProtocol: URLProtocol {
    nonisolated(unsafe) static var handler: ((URLRequest) throws -> (HTTPURLResponse, Data))?

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
    override func startLoading() {
        do {
            let (response, data) = try XCTUnwrap(Self.handler)(request)
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch {
            client?.urlProtocol(self, didFailWithError: error)
        }
    }
    override func stopLoading() {}
}
