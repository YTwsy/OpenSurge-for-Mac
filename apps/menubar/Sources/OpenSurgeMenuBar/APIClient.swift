import Foundation
import Security

enum ControlAPIError: LocalizedError {
    case descriptorUnavailable
    case tokenUnavailable
    case invalidResponse
    case http(Int)
    case keychain(OSStatus)

    var errorDescription: String? {
        switch self {
        case .descriptorUnavailable: "找不到 OpenSurge Control Service"
        case .tokenUnavailable: "找不到菜单栏客户端凭据"
        case .invalidResponse: "Control API 返回了无效数据"
        case .http(let status): "Control API 请求失败（HTTP \(status)）"
        case .keychain(let status): "无法保存菜单栏凭据到 Keychain（\(status)）"
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
        if let data = try? Data(contentsOf: url),
           let fileToken = String(data: data, encoding: .utf8), !fileToken.isEmpty {
            if KeychainStore.token() != fileToken { try KeychainStore.save(token: fileToken) }
            return fileToken
        }
        guard let keychainToken = KeychainStore.token(), !keychainToken.isEmpty else {
            throw ControlAPIError.tokenUnavailable
        }
        return keychainToken
    }

    private func request(_ url: URL, method: String, body: Data? = nil) async throws -> Data {
        var request = URLRequest(url: url)
        request.httpMethod = method
		request.httpBody = body
        request.timeoutInterval = 5
        request.setValue("Bearer \(try token())", forHTTPHeaderField: "Authorization")
		if body != nil { request.setValue("application/json", forHTTPHeaderField: "Content-Type") }
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else { throw ControlAPIError.invalidResponse }
        guard 200..<300 ~= http.statusCode else { throw ControlAPIError.http(http.statusCode) }
        return data
    }

    private func decoder() -> JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return decoder
    }
}
