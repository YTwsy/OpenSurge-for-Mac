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
            L10n.text("无法识别当前 OpenSurge 版本")
        case .invalidRelease:
            L10n.text("GitHub 返回了无效的 OpenSurge Release")
        case .networkUnavailable:
            L10n.text("无法连接 GitHub")
        case .invalidResponse:
            L10n.text("GitHub 返回了无效响应")
        case .http(let status):
            L10n.format("GitHub 请求失败（HTTP %@）", String(status))
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

func installedReleaseVersion(
    releaseTag: String?,
    shortVersion: String?
) -> String {
    let candidate = releaseTag?.trimmingCharacters(in: .whitespacesAndNewlines)
    let fallback = shortVersion?.trimmingCharacters(in: .whitespacesAndNewlines)
    let value: String
    if let candidate, !candidate.isEmpty {
        value = candidate
    } else {
        value = fallback ?? L10n.text("未知")
    }
    return value.hasPrefix("v") ? String(value.dropFirst()) : value
}

func releaseCodename(for version: String) -> String? {
    guard let parsed = ProductVersion(version) else { return nil }
    switch (parsed.major, parsed.minor) {
    case (0, 2):
        return "Wind Rose"
    default:
        return nil
    }
}

func releaseDisplayVersion(_ version: String) -> String {
    guard let codename = releaseCodename(for: version) else { return version }
    return "\(version) · \(codename)"
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
    let releaseCandidate: Int?

    init?(_ value: String) {
        let normalized = value.hasPrefix("v") ? String(value.dropFirst()) : value
        let versionParts = normalized.split(
            separator: "-",
            maxSplits: 1,
            omittingEmptySubsequences: false
        )
        guard (1...2).contains(versionParts.count) else { return nil }
        let parts = versionParts[0].split(separator: ".", omittingEmptySubsequences: false)
        guard (2...3).contains(parts.count),
              parts.allSatisfy({ !$0.isEmpty && $0.allSatisfy(\.isNumber) }),
              let major = Int(parts[0]),
              let minor = Int(parts[1]),
              let patch = parts.count == 3 ? Int(parts[2]) : 0 else {
            return nil
        }
        let releaseCandidate: Int?
        if versionParts.count == 2 {
            let prerelease = versionParts[1].split(
                separator: ".",
                omittingEmptySubsequences: false
            )
            guard prerelease.count == 2,
                  prerelease[0] == "rc",
                  !prerelease[1].isEmpty,
                  prerelease[1].allSatisfy(\.isNumber),
                  let rc = Int(prerelease[1]) else {
                return nil
            }
            releaseCandidate = rc
        } else {
            releaseCandidate = nil
        }
        self.major = major
        self.minor = minor
        self.patch = patch
        self.releaseCandidate = releaseCandidate
    }

    var description: String {
        let stable = "\(major).\(minor).\(patch)"
        return releaseCandidate.map { "\(stable)-rc.\($0)" } ?? stable
    }

    static func < (lhs: ProductVersion, rhs: ProductVersion) -> Bool {
        let lhsStable = (lhs.major, lhs.minor, lhs.patch)
        let rhsStable = (rhs.major, rhs.minor, rhs.patch)
        if lhsStable != rhsStable { return lhsStable < rhsStable }
        switch (lhs.releaseCandidate, rhs.releaseCandidate) {
        case (.some(let lhsRC), .some(let rhsRC)):
            return lhsRC < rhsRC
        case (.some, .none):
            return true
        case (.none, .some), (.none, .none):
            return false
        }
    }
}
