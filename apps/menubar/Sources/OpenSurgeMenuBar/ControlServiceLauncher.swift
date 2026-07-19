import Foundation
import Darwin

enum OpenSurgeServiceLifecycleError: LocalizedError {
    case commandFailed(String)

    var errorDescription: String? {
        switch self {
        case .commandFailed(let detail):
            detail.isEmpty ? "无法停止 OpenSurge Control Service。" : "无法停止 OpenSurge Control Service：\(detail)"
        }
    }
}

enum ControlServiceLauncher {
    private static let controlLabel = "com.opensurge.control"
    private static let coordinator = Coordinator()

    static func wake(restart: Bool = false) async {
        let uid = getuid()
        let domain = "gui/\(uid)"
        let service = "\(domain)/\(controlLabel)"
        let agent = FileManager.default.homeDirectoryForCurrentUser
            .appending(path: "Library/LaunchAgents/\(controlLabel).plist").path
        await coordinator.wake(restart: restart, service: service, domain: domain, agent: agent)
    }

    static func stopControlService() async throws {
        let service = "gui/\(getuid())/\(controlLabel)"
        try await coordinator.stop(service: service)
    }

    static func terminateMenuBarApp() -> Never {
        Darwin.exit(EXIT_SUCCESS)
    }

    private actor Coordinator {
        func wake(restart: Bool, service: String, domain: String, agent: String) {
            if runLaunchctl(["print", service]).status != 0 {
                _ = runLaunchctl(["bootstrap", domain, agent])
            }
            _ = runLaunchctl(restart ? ["kickstart", "-k", service] : ["kickstart", service])
        }

        func stop(service: String) throws {
            guard runLaunchctl(["print", service]).status == 0 else { return }
            let result = runLaunchctl(["bootout", service])
            guard result.status == 0 else { throw lifecycleError(for: result) }
        }
    }

    private static func lifecycleError(for result: CommandResult) -> OpenSurgeServiceLifecycleError {
        let detail = result.error.trimmingCharacters(in: .whitespacesAndNewlines)
        return .commandFailed(detail)
    }

    private static func runLaunchctl(_ arguments: [String]) -> CommandResult {
        runCommand("/bin/launchctl", arguments)
    }

    private static func runCommand(_ executable: String, _ arguments: [String]) -> CommandResult {
        let process = Process()
        let errorPipe = Pipe()
        process.executableURL = URL(fileURLWithPath: executable)
        process.arguments = arguments
        process.standardOutput = FileHandle.nullDevice
        process.standardError = errorPipe
        do {
            try process.run()
            process.waitUntilExit()
            let error = String(decoding: errorPipe.fileHandleForReading.readDataToEndOfFile(), as: UTF8.self)
            return CommandResult(status: process.terminationStatus, error: error)
        } catch {
            return CommandResult(status: -1, error: error.localizedDescription)
        }
    }
}

private struct CommandResult: Sendable {
    let status: Int32
    let error: String
}
