import Foundation

enum UninstallMode: Sendable {
    case keepData
    case removeEverything

    fileprivate var argument: String {
        switch self {
        case .keepData: "--keep-data"
        case .removeEverything: "--remove-all"
        }
    }
}

enum OpenSurgeUninstallError: LocalizedError {
    case componentMissing
    case authorizationCancelled
    case commandFailed(String)

    var errorDescription: String? {
        switch self {
        case .componentMissing:
            "找不到已安装的 OpenSurge 卸载组件。请重新安装当前版本后再试。"
        case .authorizationCancelled:
            "已取消卸载。"
        case .commandFailed(let detail):
            detail.isEmpty ? "无法卸载 OpenSurge。" : "无法卸载 OpenSurge：\(detail)"
        }
    }
}

enum OpenSurgeUninstaller {
    private static let scriptPath = "/Library/Application Support/OpenSurge/share/uninstall-gui.sh"

    static func run(mode: UninstallMode) async throws {
        guard FileManager.default.isExecutableFile(atPath: scriptPath) else {
            throw OpenSurgeUninstallError.componentMissing
        }

        let result = await Task.detached(priority: .userInitiated) {
            runAuthorizedScript(mode: mode)
        }.value
        guard result.status == 0 else {
            let detail = result.error.trimmingCharacters(in: .whitespacesAndNewlines)
            if detail.contains("(-128)") {
                throw OpenSurgeUninstallError.authorizationCancelled
            }
            throw OpenSurgeUninstallError.commandFailed(detail)
        }
    }

    private static func runAuthorizedScript(mode: UninstallMode) -> UninstallCommandResult {
        let process = Process()
        let outputPipe = Pipe()
        let errorPipe = Pipe()
        let command = "exec '\(scriptPath)' \(mode.argument)"
        let escapedCommand = command
            .replacingOccurrences(of: "\\", with: "\\\\")
            .replacingOccurrences(of: "\"", with: "\\\"")

        process.executableURL = URL(fileURLWithPath: "/usr/bin/osascript")
        process.arguments = [
            "-e",
            "do shell script \"\(escapedCommand)\" with administrator privileges",
        ]
        process.standardOutput = outputPipe
        process.standardError = errorPipe
        do {
            try process.run()
            process.waitUntilExit()
            let standardError = String(
                decoding: errorPipe.fileHandleForReading.readDataToEndOfFile(),
                as: UTF8.self
            )
            return UninstallCommandResult(status: process.terminationStatus, error: standardError)
        } catch {
            return UninstallCommandResult(status: -1, error: error.localizedDescription)
        }
    }
}

private struct UninstallCommandResult: Sendable {
    let status: Int32
    let error: String
}
