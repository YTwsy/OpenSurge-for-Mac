import SwiftUI

@main
@MainActor
struct OpenSurgeMenuBarApp: App {
    @NSApplicationDelegateAdaptor(OpenSurgeAppDelegate.self) private var appDelegate

    var body: some Scene {
        Settings {
            EmptyView()
        }
    }
}
