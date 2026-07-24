import AppKit
import Combine
import SwiftUI

@MainActor
protocol MenuBarPresenting: AnyObject {
    func showPanel()
}

@MainActor
final class OpenSurgeAppDelegate: NSObject, NSApplicationDelegate {
    private var presenter: (any MenuBarPresenting)?

    override init() {
        super.init()
    }

    init(presenter: any MenuBarPresenting) {
        self.presenter = presenter
        super.init()
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        if presenter == nil {
            presenter = MenuBarController(model: StatusModel())
        }

        // A user launch activates this LSUIElement app, while a login-item
        // launch normally remains inactive. Only the former should open the
        // panel automatically.
        DispatchQueue.main.async { [weak self] in
            guard NSApplication.shared.isActive else { return }
            self?.presenter?.showPanel()
        }
    }

    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        presenter?.showPanel()
        return false
    }
}

@MainActor
final class MenuBarController: NSObject, MenuBarPresenting {
    private let model: StatusModel
    private let statusItem: NSStatusItem
    private let popover = NSPopover()
    private var modelObservation: AnyCancellable?

    init(model: StatusModel) {
        self.model = model
        self.statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
        super.init()

        let contentController = NSHostingController(rootView: MenuContentView(model: model))
        contentController.sizingOptions = [.preferredContentSize]
        popover.contentViewController = contentController
        popover.behavior = .transient
        popover.animates = true

        if let button = statusItem.button {
            button.target = self
            button.action = #selector(togglePanel(_:))
            button.sendAction(on: [.leftMouseUp])
        }
        updateStatusItem()
        modelObservation = model.objectWillChange.sink { [weak self] _ in
            DispatchQueue.main.async {
                self?.updateStatusItem()
            }
        }
    }

    func showPanel() {
        guard let button = statusItem.button else { return }
        NSApplication.shared.activate(ignoringOtherApps: true)
        popover.show(relativeTo: button.bounds, of: button, preferredEdge: .minY)
    }

    @objc
    private func togglePanel(_ sender: Any?) {
        if popover.isShown {
            popover.performClose(sender)
        } else {
            showPanel()
        }
    }

    private func updateStatusItem() {
        guard let button = statusItem.button else { return }
        let indicator = model.indicator
        button.image = openSurgeMenuBarImage(for: indicator)
        button.imagePosition = .imageOnly
        button.alphaValue = indicator.menuBarIconOpacity
        button.toolTip = indicator.accessibilityLabel
        button.setAccessibilityLabel(indicator.accessibilityLabel)
    }
}
