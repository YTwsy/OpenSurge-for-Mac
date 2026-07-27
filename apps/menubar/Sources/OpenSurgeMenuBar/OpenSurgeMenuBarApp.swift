import AppKit

@main
@MainActor
enum OpenSurgeMenuBarApp {
    private static var appDelegate: OpenSurgeAppDelegate?

    static func main() {
        let application = NSApplication.shared
        let delegate = OpenSurgeAppDelegate()
        appDelegate = delegate
        application.delegate = delegate
        application.run()
    }
}
