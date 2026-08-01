import Foundation

struct AvailableUpdate: Equatable {
    let version: String
    let releasePage: URL
}

enum UpdateCheckError: LocalizedError {
    case invalidCurrentVersion
    case invalidRelease
    case networkUnavailable
    case invalidResponse
    case http(Int)

    var errorDescription: String? {
        switch self {
        case .invalidCurrentVersion:
            "无法识别当前 OpenSurge 版本"
        case .invalidRelease:
            "GitHub 返回了无效的 OpenSurge Release"
        case .networkUnavailable:
            "无法连接 GitHub"
        case .invalidResponse:
            "GitHub 返回了无效响应"
        case .http(let status):
            "GitHub 请求失败（HTTP \(status)）"
        }
    }
}

struct UpdateChecker {
    private static let latestReleaseURL = URL(
        string: "https://api.github.com/repos/YTwsy/OpenSurge-for-Mac/releases/latest"
    )!

    var session: URLSession = .shared
    var endpoint: URL = latestReleaseURL

    func check(currentVersion: String) async throws -> AvailableUpdate? {
        guard let installed = ProductVersion(currentVersion) else {
            throw UpdateCheckError.invalidCurrentVersion
        }

        var request = URLRequest(url: endpoint)
        request.timeoutInterval = 8
        request.setValue("application/vnd.github+json", forHTTPHeaderField: "Accept")
        request.setValue("2022-11-28", forHTTPHeaderField: "X-GitHub-Api-Version")
        request.setValue("OpenSurge-for-Mac/\(currentVersion)", forHTTPHeaderField: "User-Agent")

        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: request)
        } catch {
            throw UpdateCheckError.networkUnavailable
        }
        guard let http = response as? HTTPURLResponse else {
            throw UpdateCheckError.invalidResponse
        }
        guard 200..<300 ~= http.statusCode else {
            throw UpdateCheckError.http(http.statusCode)
        }

        guard let release = try? JSONDecoder().decode(GitHubRelease.self, from: data),
              !release.draft,
              !release.prerelease,
              let available = ProductVersion(release.tagName),
              trustedReleasePage(release.htmlURL, tag: release.tagName) else {
            throw UpdateCheckError.invalidRelease
        }
        guard installed < available else { return nil }
        return AvailableUpdate(version: available.description, releasePage: release.htmlURL)
    }

    private func trustedReleasePage(_ url: URL, tag: String) -> Bool {
        url.scheme == "https"
            && url.host == "github.com"
            && url.path == "/YTwsy/OpenSurge-for-Mac/releases/tag/\(tag)"
    }
}

private struct GitHubRelease: Decodable {
    let tagName: String
    let htmlURL: URL
    let draft: Bool
    let prerelease: Bool

    enum CodingKeys: String, CodingKey {
        case tagName = "tag_name"
        case htmlURL = "html_url"
        case draft, prerelease
    }
}

private struct ProductVersion: Comparable, CustomStringConvertible {
    let major: Int
    let minor: Int
    let patch: Int

    init?(_ value: String) {
        let normalized = value.hasPrefix("v") ? String(value.dropFirst()) : value
        let parts = normalized.split(separator: ".", omittingEmptySubsequences: false)
        guard (2...3).contains(parts.count),
              parts.allSatisfy({ !$0.isEmpty && $0.allSatisfy(\.isNumber) }),
              let major = Int(parts[0]),
              let minor = Int(parts[1]),
              let patch = parts.count == 3 ? Int(parts[2]) : 0 else {
            return nil
        }
        self.major = major
        self.minor = minor
        self.patch = patch
    }

    var description: String { "\(major).\(minor).\(patch)" }

    static func < (lhs: ProductVersion, rhs: ProductVersion) -> Bool {
        (lhs.major, lhs.minor, lhs.patch) < (rhs.major, rhs.minor, rhs.patch)
    }
}
