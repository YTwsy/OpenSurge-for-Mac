import AppKit
import Combine
import SwiftUI

@MainActor
protocol MenuBarPresenting: AnyObject {
    func showPanel()
    func applicationStateDidChange()
}

extension MenuBarPresenting {
    func applicationStateDidChange() {}
}

enum MenuBarPanelPresentationAction: Equatable {
    case none
    case activateApplication
    case waitForAnchor
    case showPopover
    case resetPopover
    case makePanelKey
    case complete
}

func menuBarPanelPresentationAction(
    presentationPending: Bool,
    popoverResetInProgress: Bool,
    applicationActive: Bool,
    anchorVisible: Bool,
    popoverShown: Bool,
    panelWindowAvailable: Bool,
    panelWindowKey: Bool
) -> MenuBarPanelPresentationAction {
    guard presentationPending, !popoverResetInProgress else { return .none }
    guard applicationActive else { return .activateApplication }
    guard anchorVisible else { return .waitForAnchor }
    guard popoverShown else { return .showPopover }
    guard panelWindowAvailable else { return .resetPopover }
    guard panelWindowKey else { return .makePanelKey }
    return .complete
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

        // Every process launch opens the same panel as the menu bar icon.
        // This also makes the "登录时显示" setting literal: login-item
        // launches present the panel instead of starting silently.
        presenter?.showPanel()
    }

    func applicationDidBecomeActive(_ notification: Notification) {
        presenter?.applicationStateDidChange()
    }

    func applicationDidUpdate(_ notification: Notification) {
        presenter?.applicationStateDidChange()
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
    private var presentationPending = false
    private var isResettingFailedPopover = false
    private var failedPopoverRecoveryCount = 0

    init(model: StatusModel) {
        self.model = model
        self.statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
        super.init()

        let contentController = NSHostingController(rootView: MenuContentView(model: model))
        contentController.sizingOptions = [.preferredContentSize]
        popover.contentViewController = contentController
        popover.behavior = .transient
        popover.animates = true
        popover.delegate = self

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
        presentationPending = true
        failedPopoverRecoveryCount = 0
        advancePanelPresentation()
    }

    func applicationStateDidChange() {
        advancePanelPresentation()
    }

    @objc
    private func togglePanel(_ sender: Any?) {
        if popover.isShown {
            cancelPendingPresentation()
            popover.performClose(sender)
        } else {
            showPanel()
        }
    }

    private func advancePanelPresentation() {
        guard presentationPending else { return }
        let button = statusItem.button
        let panelWindow = popover.contentViewController?.view.window
        let action = menuBarPanelPresentationAction(
            presentationPending: presentationPending,
            popoverResetInProgress: isResettingFailedPopover,
            applicationActive: NSApplication.shared.isActive,
            anchorVisible: statusItem.isVisible
                && button?.window?.isVisible == true
                && button?.isHiddenOrHasHiddenAncestor == false,
            popoverShown: popover.isShown,
            panelWindowAvailable: panelWindow != nil,
            panelWindowKey: panelWindow?.isKeyWindow == true
        )

        switch action {
        case .none, .waitForAnchor:
            return
        case .activateApplication:
            // Opening the App and clicking its status item are explicit user
            // requests to focus this LSUIElement panel. The cooperative
            // activate() API is not guaranteed to activate, so use the
            // macOS 13-compatible forced activation path here.
            NSApplication.shared.activate(ignoringOtherApps: true)
        case .showPopover:
            guard let button else { return }
            popover.show(relativeTo: button.bounds, of: button, preferredEdge: .minY)
        case .resetPopover:
            recoverFromMissingPopoverWindow()
        case .makePanelKey:
            panelWindow?.makeKey()
            if panelWindow?.isKeyWindow == true {
                completePanelPresentation()
            }
        case .complete:
            completePanelPresentation()
        }
    }

    private func recoverFromMissingPopoverWindow() {
        guard failedPopoverRecoveryCount < 3 else {
            return
        }
        failedPopoverRecoveryCount += 1
        isResettingFailedPopover = true
        popover.close()
    }

    private func completePanelPresentation() {
        presentationPending = false
        failedPopoverRecoveryCount = 0
        statusItem.button?.highlight(true)
    }

    private func cancelPendingPresentation() {
        presentationPending = false
        failedPopoverRecoveryCount = 0
        statusItem.button?.highlight(false)
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

extension MenuBarController: NSPopoverDelegate {
    func popoverDidShow(_ notification: Notification) {
        advancePanelPresentation()
    }

    func popoverDidClose(_ notification: Notification) {
        if isResettingFailedPopover {
            isResettingFailedPopover = false
            statusItem.button?.highlight(false)
            advancePanelPresentation()
            return
        }
        cancelPendingPresentation()
    }
}
