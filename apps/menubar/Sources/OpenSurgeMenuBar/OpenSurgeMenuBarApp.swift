import SwiftUI

@main
@MainActor
struct OpenSurgeMenuBarApp: App {
    @StateObject private var model = StatusModel()

    var body: some Scene {
        MenuBarExtra {
            MenuContentView(model: model)
        } label: {
            OpenSurgeMenuBarLabel(indicator: model.indicator)
                .accessibilityLabel(model.indicator.accessibilityLabel)
        }
        .menuBarExtraStyle(.window)
    }
}
