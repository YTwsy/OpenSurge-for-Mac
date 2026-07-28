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
    case waitForPanelWindow
    case resetPopover
    case makePanelKey
    case complete
}

enum MenuBarPanelRetryPolicy {
    static let presentationWindow: TimeInterval = 8
    static let popoverWindowGrace: TimeInterval = 1
    static let popoverResetGrace: TimeInterval = 0.25

    static func delay(after attempt: Int) -> TimeInterval {
        switch attempt {
        case ..<5:
            return 0.05
        case ..<10:
            return 0.1
        default:
            return 0.25
        }
    }
}

func menuBarPanelPresentationAction(
    presentationPending: Bool,
    popoverResetInProgress: Bool,
    applicationActive: Bool,
    anchorVisible: Bool,
    popoverShown: Bool,
    panelWindowAvailable: Bool,
    popoverWindowWaitExpired: Bool,
    panelWindowKey: Bool
) -> MenuBarPanelPresentationAction {
    guard presentationPending, !popoverResetInProgress else { return .none }
    guard applicationActive else { return .activateApplication }
    guard anchorVisible else { return .waitForAnchor }
    guard popoverShown else { return .showPopover }
    guard panelWindowAvailable else {
        return popoverWindowWaitExpired ? .resetPopover : .waitForPanelWindow
    }
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
    private var popover = NSPopover()
    private var modelObservation: AnyCancellable?
    private var presentationPending = false
    private var isResettingFailedPopover = false
    private var presentationRetryDeadline: Date?
    private var presentationRetryAttempt = 0
    private var presentationRetryScheduled = false
    private var popoverWindowDeadline: Date?
    private var popoverResetFallbackScheduled = false

    init(model: StatusModel) {
        self.model = model
        self.statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
        super.init()

        configurePopover(
            popover,
            contentController: makePanelContentController()
        )

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
        presentationRetryDeadline = Date().addingTimeInterval(
            MenuBarPanelRetryPolicy.presentationWindow
        )
        presentationRetryAttempt = 0
        advancePanelPresentation()
    }

    func applicationStateDidChange() {
        advancePanelPresentation()
    }

    @objc
    private func togglePanel(_ sender: Any?) {
        if popover.isShown,
           popover.contentViewController?.view.window != nil {
            cancelPendingPresentation()
            popover.performClose(sender)
        } else {
            showPanel()
        }
    }

    private func advancePanelPresentation() {
        cancelScheduledPresentationRetry()
        guard presentationPending else { return }
        let now = Date()
        let button = statusItem.button
        let panelWindow = popover.contentViewController?.view.window
        let action = menuBarPanelPresentationAction(
            presentationPending: presentationPending,
            popoverResetInProgress: isResettingFailedPopover,
            applicationActive: NSApplication.shared.isActive,
            anchorVisible: statusItem.isVisible
                && button?.window?.isVisible == true
                && button?.window?.screen != nil
                && button?.isHiddenOrHasHiddenAncestor == false,
            popoverShown: popover.isShown,
            panelWindowAvailable: panelWindow != nil,
            popoverWindowWaitExpired: popoverWindowDeadline.map { now >= $0 } ?? true,
            panelWindowKey: panelWindow?.isKeyWindow == true
        )

        switch action {
        case .none:
            return
        case .waitForAnchor:
            schedulePresentationRetry()
        case .activateApplication:
            requestApplicationActivation()
            schedulePresentationRetry()
        case .showPopover:
            guard let button else {
                schedulePresentationRetry()
                return
            }
            let windowDeadline = Date().addingTimeInterval(
                MenuBarPanelRetryPolicy.popoverWindowGrace
            )
            popoverWindowDeadline = windowDeadline
            popover.show(relativeTo: button.bounds, of: button, preferredEdge: .minY)
            schedulePresentationRetry(notBefore: windowDeadline)
        case .waitForPanelWindow:
            schedulePresentationRetry(notBefore: popoverWindowDeadline)
        case .resetPopover:
            recoverFromMissingPopoverWindow()
        case .makePanelKey:
            panelWindow?.makeKey()
            if panelWindow?.isKeyWindow == true {
                completePanelPresentation()
            } else {
                schedulePresentationRetry()
            }
        case .complete:
            completePanelPresentation()
        }
    }

    private func requestApplicationActivation() {
        // Opening the app and clicking its status item are explicit requests
        // to focus this LSUIElement panel. macOS 14 replaced forced activation
        // with cooperative activation, whose result can arrive later.
        if #available(macOS 14.0, *) {
            NSApplication.shared.activate()
        } else {
            NSApplication.shared.activate(ignoringOtherApps: true)
        }
    }

    private func recoverFromMissingPopoverWindow() {
        guard !isResettingFailedPopover else { return }
        isResettingFailedPopover = true
        popoverWindowDeadline = nil

        // A failed show can leave NSPopover logically shown without ever
        // creating a window. Replace that invalid presentation object instead
        // of depending on a didClose callback that might never arrive.
        let failedPopover = popover
        failedPopover.delegate = nil
        failedPopover.close()
        let replacement = NSPopover()
        configurePopover(
            replacement,
            contentController: makePanelContentController()
        )
        popover = replacement
        schedulePopoverResetFallback()
    }

    private func makePanelContentController() -> NSHostingController<MenuContentView> {
        let contentController = NSHostingController(
            rootView: MenuContentView(model: model)
        )
        contentController.sizingOptions = [.preferredContentSize]
        return contentController
    }

    private func configurePopover(
        _ candidate: NSPopover,
        contentController: NSViewController
    ) {
        candidate.contentViewController = contentController
        candidate.behavior = .transient
        candidate.animates = true
        candidate.delegate = self
    }

    private func schedulePresentationRetry(notBefore: Date? = nil) {
        guard presentationPending,
              !isResettingFailedPopover,
              !presentationRetryScheduled,
              let retryDeadline = presentationRetryDeadline else {
            return
        }

        let now = Date()
        let remaining = retryDeadline.timeIntervalSince(now)
        guard remaining > 0 else { return }

        var delay = MenuBarPanelRetryPolicy.delay(after: presentationRetryAttempt)
        if let notBefore {
            delay = max(delay, notBefore.timeIntervalSince(now))
        }
        delay = max(0.01, min(delay, remaining))

        presentationRetryScheduled = true
        presentationRetryAttempt += 1
        perform(
            #selector(retryPanelPresentation),
            with: nil,
            afterDelay: delay,
            inModes: [.common]
        )
    }

    @objc
    private func retryPanelPresentation() {
        presentationRetryScheduled = false
        advancePanelPresentation()
    }

    private func cancelScheduledPresentationRetry() {
        guard presentationRetryScheduled else { return }
        NSObject.cancelPreviousPerformRequests(
            withTarget: self,
            selector: #selector(retryPanelPresentation),
            object: nil
        )
        presentationRetryScheduled = false
    }

    private func schedulePopoverResetFallback() {
        cancelScheduledPopoverResetFallback()
        popoverResetFallbackScheduled = true
        perform(
            #selector(finishPopoverResetAfterTimeout),
            with: nil,
            afterDelay: MenuBarPanelRetryPolicy.popoverResetGrace,
            inModes: [.common]
        )
    }

    @objc
    private func finishPopoverResetAfterTimeout() {
        popoverResetFallbackScheduled = false
        finishPopoverReset()
    }

    private func cancelScheduledPopoverResetFallback() {
        guard popoverResetFallbackScheduled else { return }
        NSObject.cancelPreviousPerformRequests(
            withTarget: self,
            selector: #selector(finishPopoverResetAfterTimeout),
            object: nil
        )
        popoverResetFallbackScheduled = false
    }

    private func finishPopoverReset() {
        guard isResettingFailedPopover else { return }
        cancelScheduledPopoverResetFallback()
        isResettingFailedPopover = false
        statusItem.button?.highlight(false)
        schedulePresentationRetry()
    }

    private func completePanelPresentation() {
        presentationPending = false
        presentationRetryDeadline = nil
        presentationRetryAttempt = 0
        popoverWindowDeadline = nil
        cancelScheduledPresentationRetry()
        statusItem.button?.highlight(true)
    }

    private func cancelPendingPresentation() {
        presentationPending = false
        presentationRetryDeadline = nil
        presentationRetryAttempt = 0
        popoverWindowDeadline = nil
        cancelScheduledPresentationRetry()
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
        popoverWindowDeadline = nil
        advancePanelPresentation()
    }

    func popoverDidClose(_ notification: Notification) {
        if isResettingFailedPopover {
            finishPopoverReset()
            return
        }
        cancelPendingPresentation()
    }
}
