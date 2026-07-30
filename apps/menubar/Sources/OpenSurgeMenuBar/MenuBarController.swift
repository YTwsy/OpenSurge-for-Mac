import AppKit
import Combine
import SwiftUI

@MainActor
protocol MenuBarPresenting: AnyObject {
    func showPanel()
    func applicationDidBecomeActive()
}

extension MenuBarPresenting {
    func applicationDidBecomeActive() {}
}

enum MenuBarPanelPresentationAction: Equatable {
    case none
    case waitForAnchor
    case showPopover
    case waitForPanelWindow
    case waitForPopoverAnimation
    case replacePopover
    case complete
}

enum MenuBarPanelFocusAction: Equatable {
    case none
    case deferUntilPopoverDidShow
    case promote
}

enum MenuBarPanelRetryPolicy {
    static let presentationWindow: TimeInterval = 8
    static let popoverWindowGrace: TimeInterval = 1
    static let popoverShowAnimationGrace: TimeInterval = 0.5

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

enum MenuBarPanelFocusPolicy {
    static let cooperativeActivationGrace: TimeInterval = 0.05
}

func menuBarPanelPresentationAction(
    presentationPending: Bool,
    anchorVisible: Bool,
    popoverShown: Bool,
    panelWindowAvailable: Bool,
    popoverWindowWaitExpired: Bool,
    popoverAnimationFinished: Bool,
    popoverAnimationWaitExpired: Bool
) -> MenuBarPanelPresentationAction {
    guard presentationPending else { return .none }
    guard anchorVisible else { return .waitForAnchor }
    guard popoverShown else { return .showPopover }
    guard panelWindowAvailable else {
        return popoverWindowWaitExpired ? .replacePopover : .waitForPanelWindow
    }
    // The popover window exists from the moment show() starts animating, so it
    // only proves visibility began. Completing here would let focus work run
    // inside the unfinished expand animation.
    guard popoverAnimationFinished || popoverAnimationWaitExpired else {
        return .waitForPopoverAnimation
    }
    return .complete
}

func menuBarPopoverBehavior(applicationActive: Bool) -> NSPopover.Behavior {
    applicationActive ? .transient : .applicationDefined
}

func menuBarPanelFocusAction(
    panelWindowAvailable: Bool,
    applicationActive: Bool,
    popoverShowAnimationInFlight: Bool
) -> MenuBarPanelFocusAction {
    guard panelWindowAvailable, applicationActive else { return .none }
    return popoverShowAnimationInFlight ? .deferUntilPopoverDidShow : .promote
}

func menuBarNeedsForcedApplicationActivation(
    panelPresented: Bool,
    applicationActive: Bool
) -> Bool {
    panelPresented && !applicationActive
}

func menuBarStatusItemNeedsRefresh(
    renderedIndicator: IndicatorState?,
    nextIndicator: IndicatorState
) -> Bool {
    renderedIndicator != nextIndicator
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
        presenter?.applicationDidBecomeActive()
    }

    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        // A LaunchServices reopen can arrive while this LSUIElement process is
        // still inactive. Force the activation requested by the user's click,
        // then let AppKit complete that transition before starting the
        // NSPopover animation on the next main-run-loop turn.
        sender.activate(ignoringOtherApps: true)
        DispatchQueue.main.async { [weak self] in
            self?.presenter?.showPanel()
        }
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
    private var presentationRetryDeadline: Date?
    private var presentationRetryAttempt = 0
    private var presentationRetryScheduled = false
    private var popoverWindowDeadline: Date?
    private var popoverShowAnimationDeadline: Date?
    private var awaitingPopoverShowAnimation = false
    private var panelPresented = false
    private var renderedIndicator: IndicatorState?
    private var outsideClickMonitor: Any?
    private var escapeKeyMonitor: Any?
    private var finalPresentationVerificationScheduled = false
    private var applicationActivationFallbackScheduled = false

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
            // NSChangeBackgroundCellMask is imported without a Swift member
            // name in the macOS 14 SDK used by this project.
            (button.cell as? NSButtonCell)?.showsStateBy = NSCell.StyleMask(rawValue: 1 << 3)
        }
        updateStatusItem()
        modelObservation = model.objectWillChange.sink { [weak self] _ in
            DispatchQueue.main.async {
                self?.updateStatusItem()
            }
        }
    }

    func showPanel() {
        cancelFinalPresentationVerification()
        presentationPending = true
        presentationRetryDeadline = Date().addingTimeInterval(
            MenuBarPanelRetryPolicy.presentationWindow
        )
        presentationRetryAttempt = 0
        requestApplicationActivation()
        advancePanelPresentation()
    }

    func applicationDidBecomeActive() {
        cancelApplicationActivationFallback()
        advancePanelPresentation()
        promoteVisiblePanelForActiveApplication()
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
            anchorVisible: statusItem.isVisible
                && button?.window?.isVisible == true
                && button?.window?.screen != nil
                && button?.isHiddenOrHasHiddenAncestor == false,
            popoverShown: popover.isShown,
            panelWindowAvailable: panelWindow != nil,
            popoverWindowWaitExpired: popoverWindowDeadline.map { now >= $0 } ?? true,
            popoverAnimationFinished: !awaitingPopoverShowAnimation,
            popoverAnimationWaitExpired: popoverShowAnimationDeadline
                .map { now >= $0 } ?? true
        )

        switch action {
        case .none:
            return
        case .waitForAnchor:
            schedulePresentationRetry()
        case .showPopover:
            guard let button else {
                schedulePresentationRetry()
                return
            }
            showPopover(anchoredTo: button)
            schedulePresentationRetry(notBefore: popoverWindowDeadline)
        case .waitForPanelWindow:
            schedulePresentationRetry(notBefore: popoverWindowDeadline)
        case .waitForPopoverAnimation:
            // popoverDidShow normally advances this. The bounded retry only
            // guarantees the state machine cannot stay pending if AppKit never
            // reports the animation as finished.
            schedulePresentationRetry(notBefore: popoverShowAnimationDeadline)
        case .replacePopover:
            replaceFailedPopover()
            schedulePresentationRetry()
        case .complete:
            completePanelPresentation()
        }
    }

    private func requestApplicationActivation() {
        // Activation is a best-effort enhancement. Presentation never waits
        // for this request because cooperative activation may be declined.
        if #available(macOS 14.0, *) {
            NSApplication.shared.activate()
        } else {
            NSApplication.shared.activate(ignoringOtherApps: true)
        }
    }

    private func scheduleApplicationActivationFallback() {
        // A forced activation transition lands in the middle of the expand
        // animation otherwise, which is exactly what leaves the panel half open.
        guard !awaitingPopoverShowAnimation,
        menuBarNeedsForcedApplicationActivation(
            panelPresented: panelPresented,
            applicationActive: NSApplication.shared.isActive
        ),
        !applicationActivationFallbackScheduled else {
            return
        }

        applicationActivationFallbackScheduled = true
        perform(
            #selector(forceApplicationActivationIfNeeded),
            with: nil,
            afterDelay: MenuBarPanelFocusPolicy.cooperativeActivationGrace,
            inModes: [.common]
        )
    }

    @objc
    private func forceApplicationActivationIfNeeded() {
        applicationActivationFallbackScheduled = false
        guard !awaitingPopoverShowAnimation,
        menuBarNeedsForcedApplicationActivation(
            panelPresented: panelPresented,
            applicationActive: NSApplication.shared.isActive
        ),
        popover.isShown,
        let panelWindow = popover.contentViewController?.view.window else {
            return
        }

        // macOS 26 can decline cooperative activation for an LSUIElement app
        // even after an explicitly requested popover has a real window. Keep
        // this deprecated call as a narrow, one-shot fallback after visibility
        // succeeds; it must never become a presentation prerequisite.
        NSApplication.shared.activate(ignoringOtherApps: true)
        panelWindow.makeKeyAndOrderFront(nil)
        promoteVisiblePanelForActiveApplication()
    }

    private func cancelApplicationActivationFallback() {
        guard applicationActivationFallbackScheduled else { return }
        NSObject.cancelPreviousPerformRequests(
            withTarget: self,
            selector: #selector(forceApplicationActivationIfNeeded),
            object: nil
        )
        applicationActivationFallbackScheduled = false
    }

    private func showPopover(anchoredTo button: NSButton) {
        preparePopoverForCurrentActivationState()
        popoverWindowDeadline = Date().addingTimeInterval(
            MenuBarPanelRetryPolicy.popoverWindowGrace
        )
        // Only popoverDidShow reports that the expand animation finished, so
        // assume it is in flight until then and give it a bounded grace period.
        awaitingPopoverShowAnimation = popover.animates
        popoverShowAnimationDeadline = popover.animates
            ? Date().addingTimeInterval(
                MenuBarPanelRetryPolicy.popoverShowAnimationGrace
            )
            : nil
        popover.show(relativeTo: button.bounds, of: button, preferredEdge: .minY)
    }

    private func clearPopoverShowAnimationState() {
        awaitingPopoverShowAnimation = false
        popoverShowAnimationDeadline = nil
        popoverWindowDeadline = nil
    }

    private func replaceFailedPopover() {
        clearPopoverShowAnimationState()
        setPanelPresented(false)
        removeApplicationDefinedDismissMonitors()

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
    }

    private func makePanelContentController() -> NSViewController {
        let contentController = NSHostingController(
            rootView: MenuContentView(model: model)
                .environment(\.controlActiveState, .key)
        )
        contentController.sizingOptions = [.preferredContentSize]
        return contentController
    }

    private func configurePopover(
        _ candidate: NSPopover,
        contentController: NSViewController
    ) {
        candidate.contentViewController = contentController
        candidate.behavior = .applicationDefined
        candidate.animates = true
        candidate.delegate = self
    }

    private func preparePopoverForCurrentActivationState() {
        popover.behavior = menuBarPopoverBehavior(
            applicationActive: NSApplication.shared.isActive
        )
        if popover.behavior == .transient {
            removeApplicationDefinedDismissMonitors()
        } else {
            installApplicationDefinedDismissMonitors()
        }
    }

    private func installApplicationDefinedDismissMonitors() {
        if outsideClickMonitor == nil {
            outsideClickMonitor = NSEvent.addGlobalMonitorForEvents(
                matching: [.leftMouseDown, .rightMouseDown, .otherMouseDown]
            ) { [weak self] _ in
                DispatchQueue.main.async {
                    self?.closePanel()
                }
            }
        }
        if escapeKeyMonitor == nil {
            escapeKeyMonitor = NSEvent.addLocalMonitorForEvents(matching: .keyDown) {
                [weak self] event in
                guard event.keyCode == 53 else { return event }
                self?.closePanel()
                return nil
            }
        }
    }

    private func removeApplicationDefinedDismissMonitors() {
        if let outsideClickMonitor {
            NSEvent.removeMonitor(outsideClickMonitor)
            self.outsideClickMonitor = nil
        }
        if let escapeKeyMonitor {
            NSEvent.removeMonitor(escapeKeyMonitor)
            self.escapeKeyMonitor = nil
        }
    }

    private func promoteVisiblePanelForActiveApplication() {
        let panelWindow = popover.contentViewController?.view.window
        // Keying, reordering or re-targeting dismissal for a popover window that
        // AppKit is still animating open stops the expansion at an intermediate
        // frame until the next input event resumes it. popoverDidShow replays
        // this promotion once the animation really finished.
        guard menuBarPanelFocusAction(
            panelWindowAvailable: panelWindow != nil,
            applicationActive: NSApplication.shared.isActive,
            popoverShowAnimationInFlight: awaitingPopoverShowAnimation
        ) == .promote, let panelWindow else {
            return
        }
        panelWindow.makeKeyAndOrderFront(nil)
        popover.behavior = .transient
        removeApplicationDefinedDismissMonitors()
    }

    private func schedulePresentationRetry(notBefore: Date? = nil) {
        guard presentationPending,
              !presentationRetryScheduled,
              let retryDeadline = presentationRetryDeadline else {
            return
        }

        let now = Date()
        let remaining = retryDeadline.timeIntervalSince(now)
        guard remaining > 0 else {
            finishTimedOutPanelPresentation()
            return
        }

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

    private func finishTimedOutPanelPresentation() {
        let button = statusItem.button
        let anchorVisible = statusItem.isVisible
            && button?.window?.isVisible == true
            && button?.window?.screen != nil
            && button?.isHiddenOrHasHiddenAncestor == false

        if popover.contentViewController?.view.window != nil {
            completePanelPresentation()
            return
        }
        if popover.isShown {
            replaceFailedPopover()
        }

        clearPendingPresentation()
        guard anchorVisible, let button else {
            setPanelPresented(false)
            return
        }

        // One final non-blocking attempt avoids leaving a pending state behind.
        // If AppKit still declines it, the next explicit click starts fresh.
        showPopover(anchoredTo: button)
        if popover.contentViewController?.view.window != nil,
           !awaitingPopoverShowAnimation {
            completePanelPresentation()
        } else {
            scheduleFinalPresentationVerification()
        }
    }

    private func completePanelPresentation() {
        cancelFinalPresentationVerification()
        clearPendingPresentation()
        // Either popoverDidShow arrived or its bounded grace expired, so focus
        // work must no longer be deferred.
        clearPopoverShowAnimationState()
        setPanelPresented(true)
        promoteVisiblePanelForActiveApplication()
        scheduleApplicationActivationFallback()
    }

    private func cancelPendingPresentation() {
        cancelFinalPresentationVerification()
        clearPendingPresentation()
        clearPopoverShowAnimationState()
        setPanelPresented(false)
        removeApplicationDefinedDismissMonitors()
    }

    private func scheduleFinalPresentationVerification() {
        cancelFinalPresentationVerification()
        finalPresentationVerificationScheduled = true
        perform(
            #selector(verifyFinalPanelPresentation),
            with: nil,
            afterDelay: MenuBarPanelRetryPolicy.popoverWindowGrace,
            inModes: [.common]
        )
    }

    @objc
    private func verifyFinalPanelPresentation() {
        finalPresentationVerificationScheduled = false
        if popover.contentViewController?.view.window != nil {
            completePanelPresentation()
            return
        }
        if popover.isShown {
            replaceFailedPopover()
        }
        setPanelPresented(false)
        removeApplicationDefinedDismissMonitors()
    }

    private func cancelFinalPresentationVerification() {
        guard finalPresentationVerificationScheduled else { return }
        NSObject.cancelPreviousPerformRequests(
            withTarget: self,
            selector: #selector(verifyFinalPanelPresentation),
            object: nil
        )
        finalPresentationVerificationScheduled = false
    }

    private func clearPendingPresentation() {
        presentationPending = false
        presentationRetryDeadline = nil
        presentationRetryAttempt = 0
        cancelScheduledPresentationRetry()
    }

    private func closePanel() {
        cancelPendingPresentation()
        if popover.isShown {
            popover.performClose(nil)
        }
    }

    private func setPanelPresented(_ presented: Bool) {
        panelPresented = presented
        if !presented {
            cancelApplicationActivationFallback()
        }
        applyStatusItemPresentedState()
    }

    private func applyStatusItemPresentedState() {
        guard let button = statusItem.button else { return }
        button.state = panelPresented ? .on : .off
    }

    private func updateStatusItem() {
        guard let button = statusItem.button else { return }
        let indicator = model.indicator
        guard menuBarStatusItemNeedsRefresh(
            renderedIndicator: renderedIndicator,
            nextIndicator: indicator
        ) else {
            applyStatusItemPresentedState()
            return
        }
        renderedIndicator = indicator
        button.image = openSurgeMenuBarImage(for: indicator)
        button.imagePosition = .imageOnly
        button.alphaValue = indicator.menuBarIconOpacity
        button.toolTip = indicator.accessibilityLabel
        button.setAccessibilityLabel(indicator.accessibilityLabel)
        applyStatusItemPresentedState()
    }
}

extension MenuBarController: NSPopoverDelegate {
    func popoverDidShow(_ notification: Notification) {
        // This is the only AppKit signal that the expand animation finished.
        clearPopoverShowAnimationState()
        guard presentationPending else {
            // Presentation already settled through its bounded grace period, so
            // run the focus work that was deferred during the animation.
            promoteVisiblePanelForActiveApplication()
            scheduleApplicationActivationFallback()
            return
        }
        advancePanelPresentation()
    }

    func popoverDidClose(_ notification: Notification) {
        cancelPendingPresentation()
    }
}
