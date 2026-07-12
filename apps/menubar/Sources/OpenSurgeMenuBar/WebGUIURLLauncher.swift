import AppKit
import Foundation

enum WebGUIURLLaunchError: LocalizedError {
    case browserUnavailable

    var errorDescription: String? {
        switch self {
        case .browserUnavailable:
            return "Web GUI 链接已生成，但 macOS 未能交给默认浏览器。请检查默认浏览器后重试。"
        }
    }
}

struct WebGUIURLLauncher {
    private let workspaceOpen: (URL) -> Bool
    private let commandOpen: (URL) throws -> Void

    init(
        workspaceOpen: @escaping (URL) -> Bool = { NSWorkspace.shared.open($0) },
        commandOpen: @escaping (URL) throws -> Void = WebGUIURLLauncher.openWithSystemCommand
    ) {
        self.workspaceOpen = workspaceOpen
        self.commandOpen = commandOpen
    }

    func open(_ url: URL) throws {
        if workspaceOpen(url) { return }

        do {
            try commandOpen(url)
        } catch {
            throw WebGUIURLLaunchError.browserUnavailable
        }
    }

    private static func openWithSystemCommand(_ url: URL) throws {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/usr/bin/open")
        process.arguments = [url.absoluteString]
        try process.run()
        process.waitUntilExit()
        guard process.terminationStatus == 0 else {
            throw WebGUIURLLaunchError.browserUnavailable
        }
    }
}
