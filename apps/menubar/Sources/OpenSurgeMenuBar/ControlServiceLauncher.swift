import Foundation
import Darwin

enum ControlServiceLauncher {
    static func wake(restart: Bool = false) async {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/bin/launchctl")
        process.arguments = restart
            ? ["kickstart", "-k", "gui/\(getuid())/com.opensurge.control"]
            : ["kickstart", "gui/\(getuid())/com.opensurge.control"]
        process.standardOutput = FileHandle.nullDevice
        process.standardError = FileHandle.nullDevice
        try? process.run()
        process.waitUntilExit()
    }
}
