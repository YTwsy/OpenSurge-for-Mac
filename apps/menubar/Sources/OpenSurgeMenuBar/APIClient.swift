import Foundation

enum ControlAPIError: LocalizedError {
    case descriptorUnavailable
    case tokenUnavailable
    case transportUnavailable
    case invalidResponse
    case http(Int)

    var serviceUnavailable: Bool {
        switch self {
        case .descriptorUnavailable, .tokenUnavailable, .transportUnavailable: true
        case .invalidResponse, .http: false
        }
    }

    var errorDescription: String? {
        switch self {
        case .descriptorUnavailable, .tokenUnavailable: "OpenSurge 后台服务尚未准备好"
        case .transportUnavailable: "无法连接 OpenSurge 后台服务"
        case .invalidResponse: "Control API 返回了无效数据"
        case .http(let status): "Control API 请求失败（HTTP \(status)）"
        }
    }
}

struct ControlAPIClient {
    var session: URLSession = .shared
    var applicationSupport: URL = FileManager.default.homeDirectoryForCurrentUser
        .appending(path: "Library/Application Support/OpenSurge", directoryHint: .isDirectory)
    var tokenOverride: String?

    func status() async throws -> MenuBarStatus {
        let endpoint = try descriptor().url.appending(path: "api/v1/menubar")
        let data = try await request(endpoint, method: "GET")
        return try decoder().decode(MenuBarStatus.self, from: data)
    }

    func bootstrapURL(path: String) async throws -> URL {
        let endpoint = try descriptor().url.appending(path: "api/v1/session/bootstrap")
        let body = try JSONEncoder().encode(["path": path])
        let data = try await request(endpoint, method: "POST", body: body)
        return try decoder().decode(BootstrapResponse.self, from: data).url
    }

    private func descriptor() throws -> EndpointDescriptor {
        let url = applicationSupport.appending(path: "control-endpoint.json")
        guard let data = try? Data(contentsOf: url),
              let value = try? JSONDecoder().decode(EndpointDescriptor.self, from: data) else {
            throw ControlAPIError.descriptorUnavailable
        }
        return value
    }

    private func token() throws -> String {
        if let tokenOverride { return tokenOverride }
        let url = applicationSupport.appending(path: "control-token")
        guard let data = try? Data(contentsOf: url),
              let fileToken = String(data: data, encoding: .utf8),
              !fileToken.isEmpty else {
            throw ControlAPIError.tokenUnavailable
        }
        return fileToken
    }

    private func request(_ url: URL, method: String, body: Data? = nil) async throws -> Data {
        var request = URLRequest(url: url)
        request.httpMethod = method
		request.httpBody = body
        request.timeoutInterval = 5
        request.setValue("Bearer \(try token())", forHTTPHeaderField: "Authorization")
		if body != nil { request.setValue("application/json", forHTTPHeaderField: "Content-Type") }
        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: request)
        } catch is URLError {
            throw ControlAPIError.transportUnavailable
        }
        guard let http = response as? HTTPURLResponse else { throw ControlAPIError.invalidResponse }
        guard 200..<300 ~= http.statusCode else { throw ControlAPIError.http(http.statusCode) }
        return data
    }

    private func decoder() -> JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .custom { decoder in
            let container = try decoder.singleValueContainer()
            let value = try container.decode(String.self)
            let formatter = ISO8601DateFormatter()

            formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            if let date = formatter.date(from: value) { return date }

            formatter.formatOptions = [.withInternetDateTime]
            if let date = formatter.date(from: value) { return date }

            throw DecodingError.dataCorruptedError(
                in: container,
                debugDescription: "Expected RFC3339 timestamp with or without fractional seconds"
            )
        }
        return decoder
    }
}
