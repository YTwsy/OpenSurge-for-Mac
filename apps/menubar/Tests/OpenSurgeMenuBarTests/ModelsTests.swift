import XCTest
@testable import OpenSurgeMenuBar

final class ModelsTests: XCTestCase {
    func testRecoveryHasHighestIndicatorPriority() {
        let status = MenuBarStatus(schemaVersion: 1, revision: "r", gateway: "stopped", topology: "same_wifi_dhcp",
                                   lanIp: "192.168.1.20", dhcp: "stopped", mihomo: "stopped", pfAnchor: "unloaded",
                                   forwarding: "disabled", clientCount: 0, drift: true, doctorHealthy: false,
                                   recoveryRequired: true, recoveryStage: "gateway_stopped_waiting_router_dhcp", warnings: [], errorCode: nil)
        XCTAssertEqual(status.indicator, .recovery)
    }

    func testPreparedRecoveryCardDoesNotImplyNetworkHasChanged() {
        let status = MenuBarStatus(schemaVersion: 1, revision: "r", gateway: "stopped", topology: "same_wifi_dhcp",
                                   lanIp: "192.168.1.20", dhcp: "stopped", mihomo: "stopped", pfAnchor: "unloaded",
                                   forwarding: "disabled", clientCount: 0, drift: false, doctorHealthy: true,
                                   recoveryRequired: true, recoveryStage: "prepared", warnings: [], errorCode: nil)
        XCTAssertTrue(status.recoverySnapshotPrepared)
        XCTAssertFalse(status.recoveryNeedsAttention)
        XCTAssertEqual(status.indicator, .stopped)
    }

    func testDriftIsDegraded() {
        XCTAssertEqual(fixture(gateway: "running", recovery: false, drift: true).indicator, .degraded)
    }

    func testStoppedGatewayIsNotReportedAsRuntimeFailure() {
        let status = MenuBarStatus(schemaVersion: 1, revision: "r", gateway: "stopped", topology: "same_wifi_dhcp",
                                   lanIp: "192.168.1.20", dhcp: "stopped", mihomo: "stopped", pfAnchor: "unloaded",
                                   forwarding: "disabled", clientCount: 0, drift: true, doctorHealthy: false,
                                   recoveryRequired: false, recoveryStage: nil, warnings: [], errorCode: nil)
        XCTAssertEqual(status.indicator, .stopped)
        XCTAssertEqual(status.indicator.accessibilityLabel, "OpenSurge 网关已停止")
        XCTAssertTrue(status.indicator.usesBrandMenuBarIcon)
        XCTAssertEqual(status.indicator.menuBarIconOpacity, 0.55)
    }

    func testActiveTakeoverUsesRunningIndicatorInsteadOfRecovery() {
        let status = MenuBarStatus(schemaVersion: 1, revision: "r", gateway: "running", topology: "same_wifi_dhcp",
                                   lanIp: "192.168.1.20", dhcp: "running", mihomo: "running", pfAnchor: "loaded",
                                   forwarding: "enabled", clientCount: 2, drift: false, doctorHealthy: true,
                                   recoveryRequired: true, recoveryStage: "gateway_active", warnings: [], errorCode: nil)
        XCTAssertTrue(status.takeoverActive)
        XCTAssertFalse(status.recoveryNeedsAttention)
        XCTAssertEqual(status.indicator, .running)
        XCTAssertTrue(status.indicator.usesBrandMenuBarIcon)
        XCTAssertEqual(status.indicator.menuBarIconOpacity, 1)
    }

    func testAlertStatesKeepSystemStatusSymbols() {
        for state in [IndicatorState.degraded, .recovery] {
            XCTAssertFalse(state.usesBrandMenuBarIcon)
            XCTAssertEqual(state.menuBarIconOpacity, 1)
        }
        XCTAssertEqual(IndicatorState.recovery.systemImage, "exclamationmark.triangle.fill")
    }

    func testInitialConnectionUsesBrandIconUntilARealFailureIsKnown() {
        XCTAssertEqual(menuBarIndicator(status: nil, hasError: false), .connecting)
        XCTAssertTrue(IndicatorState.connecting.usesBrandMenuBarIcon)
        XCTAssertEqual(IndicatorState.connecting.menuBarIconOpacity, 0.75)
        XCTAssertEqual(IndicatorState.connecting.accessibilityLabel, "正在连接 OpenSurge 控制服务")

        XCTAssertEqual(menuBarIndicator(status: nil, hasError: true), .unreachable)
        XCTAssertTrue(IndicatorState.unreachable.usesBrandMenuBarIcon)
        XCTAssertEqual(IndicatorState.unreachable.menuBarIconOpacity, 0.35)
        XCTAssertEqual(IndicatorState.unreachable.accessibilityLabel, "无法连接 OpenSurge 控制服务")
    }

    func testSkippedClientAcceptanceRemainsAnActiveTakeover() {
        let status = MenuBarStatus(schemaVersion: 1, revision: "r", gateway: "running", topology: "same_wifi_dhcp",
                                   lanIp: "192.168.1.20", dhcp: "running", mihomo: "running", pfAnchor: "loaded",
                                   forwarding: "enabled", clientCount: 0, drift: false, doctorHealthy: true,
                                   recoveryRequired: true, recoveryStage: "client_validation_skipped", warnings: [], errorCode: nil)
        XCTAssertTrue(status.takeoverActive)
        XCTAssertFalse(status.recoveryNeedsAttention)
        XCTAssertEqual(status.indicator, .running)
    }

    func testQuitWarningDistinguishesGatewayFromMenuBarProcess() {
        let active = fixture(gateway: "running", recovery: false, drift: false)
        XCTAssertTrue(active.gatewayServicesActive)
        XCTAssertTrue(menuBarQuitWarning(for: active).contains("都不会停止"))
        let stopped = MenuBarStatus(schemaVersion: 1, revision: "r", gateway: "stopped", topology: "same_wifi_dhcp",
                                    lanIp: "192.168.1.20", dhcp: "stopped", mihomo: "stopped", pfAnchor: "unloaded",
                                    forwarding: "disabled", clientCount: 0, drift: false, doctorHealthy: true,
                                    recoveryRequired: false, recoveryStage: nil, warnings: [], errorCode: nil)
        XCTAssertFalse(stopped.gatewayServicesActive)
        XCTAssertTrue(menuBarQuitWarning(for: stopped).contains("后台控制服务仍会继续运行"))
        XCTAssertTrue(menuBarQuitWarning(for: nil).contains("无法确认网关服务状态"))
    }

    func testFullQuitRequiresStoppedGatewayAndNoPendingRecovery() {
        let active = fixture(gateway: "running", recovery: false, drift: false)
        XCTAssertFalse(active.canQuitOpenSurge)

        let stopped = MenuBarStatus(schemaVersion: 1, revision: "r", gateway: "stopped", topology: "isolated_lan",
                                    lanIp: "192.168.50.1", dhcp: "stopped", mihomo: "stopped", pfAnchor: "unloaded",
                                    forwarding: "disabled", clientCount: 0, drift: false, doctorHealthy: true,
                                    recoveryRequired: false, recoveryStage: nil, warnings: [], errorCode: nil)
        XCTAssertTrue(stopped.canQuitOpenSurge)
        XCTAssertTrue(openSurgeQuitWarning(for: stopped).contains("root Helper 仍保持空闲加载"))
        XCTAssertTrue(openSurgeQuitWarning(for: stopped).contains("不需要再次授权"))

        let recovery = MenuBarStatus(schemaVersion: 1, revision: "r", gateway: "stopped", topology: "same_wifi_dhcp",
                                     lanIp: "192.168.1.20", dhcp: "stopped", mihomo: "stopped", pfAnchor: "unloaded",
                                     forwarding: "disabled", clientCount: 0, drift: false, doctorHealthy: true,
                                     recoveryRequired: true, recoveryStage: "gateway_stopped_waiting_router_dhcp", warnings: [], errorCode: nil)
        XCTAssertFalse(recovery.canQuitOpenSurge)
        XCTAssertTrue(openSurgeQuitWarning(for: recovery).contains("网络恢复尚未完成"))
        XCTAssertTrue(recovery.canUninstall)
        XCTAssertTrue(uninstallWarning(for: recovery).contains("IPv4 forwarding"))

        let abandoned = MenuBarStatus(schemaVersion: 1, revision: "r", gateway: "stopped", topology: "same_wifi_dhcp",
                                      lanIp: "192.168.1.20", dhcp: "stopped", mihomo: "stopped", pfAnchor: "unloaded",
                                      forwarding: "disabled", clientCount: 0, drift: false, doctorHealthy: true,
                                      recoveryRequired: false, recoveryStage: "complete_static", warnings: [], errorCode: nil)
        XCTAssertTrue(abandoned.canQuitOpenSurge)
    }

    func testHostForwardingDoesNotKeepOpenSurgeActive() {
        let stopped = MenuBarStatus(schemaVersion: 1, revision: "r", gateway: "stopped", topology: "isolated_lan",
                                    lanIp: "192.168.50.1", dhcp: "stopped", mihomo: "stopped", pfAnchor: "unloaded",
                                    forwarding: "enabled", clientCount: 0, drift: false, doctorHealthy: true,
                                    recoveryRequired: false, recoveryStage: nil, warnings: [], errorCode: nil)

        XCTAssertFalse(stopped.gatewayServicesActive)
        XCTAssertTrue(stopped.canQuitOpenSurge)
        XCTAssertTrue(stopped.canUninstall)
    }

    func testUninstallOnlyDependsOnGatewayBeingStopped() {
        XCTAssertFalse(fixture(gateway: "running", recovery: false, drift: false).canUninstall)
        XCTAssertFalse(fixture(gateway: "degraded", recovery: false, drift: false).canUninstall)
        XCTAssertTrue(uninstallWarning(for: fixture(gateway: "running", recovery: false, drift: false)).contains("停止网关"))
    }

    func testDiagnosticSummaryDoesNotContainWarnings() {
        let status = fixture(gateway: "running", recovery: false, drift: false)
        XCTAssertFalse(status.diagnosticSummary.contains("secret-value"))
    }

    func testSameWiFiTechnicalModeUsesDHCPTakeoverProductLabel() {
        XCTAssertEqual(fixture(gateway: "stopped", recovery: false, drift: false).topologyLabel, "局域网 DHCP 接管")
    }

    @MainActor
    func testReopeningAppShowsMenuBarPanel() {
        let presenter = RecordingMenuBarPresenter()
        let delegate = OpenSurgeAppDelegate(presenter: presenter)

        XCTAssertFalse(delegate.applicationShouldHandleReopen(NSApplication.shared, hasVisibleWindows: false))
        let presentationCount = presenter.presentationCount
        XCTAssertEqual(presentationCount, 1)
    }

    @MainActor
    func testApplicationLifecycleEventsResumePanelPresentation() {
        let presenter = RecordingMenuBarPresenter()
        let delegate = OpenSurgeAppDelegate(presenter: presenter)

        delegate.applicationDidBecomeActive(
            Notification(name: NSApplication.didBecomeActiveNotification)
        )

        XCTAssertEqual(presenter.stateChangeCount, 1)
    }

    func testPanelPresentationRequiresVisibleAnchorAndRealWindowButNotActivationOrKeyWindow() {
        XCTAssertEqual(panelPresentationAction(anchorVisible: false), .waitForAnchor)
        XCTAssertEqual(panelPresentationAction(popoverShown: false), .showPopover)
        XCTAssertEqual(
            panelPresentationAction(
                panelWindowAvailable: false,
                popoverWindowWaitExpired: false
            ),
            .waitForPanelWindow
        )
        XCTAssertEqual(
            panelPresentationAction(
                panelWindowAvailable: false,
                popoverWindowWaitExpired: true
            ),
            .replacePopover
        )
        XCTAssertEqual(panelPresentationAction(), .complete)
        XCTAssertEqual(
            menuBarPanelPresentationAction(
                presentationPending: false,
                anchorVisible: true,
                popoverShown: true,
                panelWindowAvailable: true,
                popoverWindowWaitExpired: true
            ),
            .none
        )
    }

    func testPanelPresentationRetriesAreBoundedAndBackOff() {
        XCTAssertEqual(MenuBarPanelRetryPolicy.delay(after: 0), 0.05)
        XCTAssertEqual(MenuBarPanelRetryPolicy.delay(after: 4), 0.05)
        XCTAssertEqual(MenuBarPanelRetryPolicy.delay(after: 5), 0.1)
        XCTAssertEqual(MenuBarPanelRetryPolicy.delay(after: 9), 0.1)
        XCTAssertEqual(MenuBarPanelRetryPolicy.delay(after: 10), 0.25)
        XCTAssertGreaterThan(
            MenuBarPanelRetryPolicy.presentationWindow,
            MenuBarPanelRetryPolicy.popoverWindowGrace
        )
    }

    func testInactivePopoverOwnsDismissalAndStatusRefreshIsDiffed() {
        XCTAssertEqual(
            menuBarPopoverBehavior(applicationActive: false),
            .applicationDefined
        )
        XCTAssertEqual(
            menuBarPopoverBehavior(applicationActive: true),
            .transient
        )
        XCTAssertFalse(
            menuBarStatusItemNeedsRefresh(
                renderedIndicator: .running,
                nextIndicator: .running
            )
        )
        XCTAssertTrue(
            menuBarStatusItemNeedsRefresh(
                renderedIndicator: .running,
                nextIndicator: .degraded
            )
        )
    }

    func testVisibleInactivePanelNeedsOneShotActivationFallback() {
        XCTAssertTrue(
            menuBarNeedsForcedApplicationActivation(
                panelPresented: true,
                applicationActive: false
            )
        )
        XCTAssertFalse(
            menuBarNeedsForcedApplicationActivation(
                panelPresented: false,
                applicationActive: false
            )
        )
        XCTAssertFalse(
            menuBarNeedsForcedApplicationActivation(
                panelPresented: true,
                applicationActive: true
            )
        )
        XCTAssertGreaterThan(
            MenuBarPanelFocusPolicy.cooperativeActivationGrace,
            0
        )
    }

    private func fixture(gateway: String, recovery: Bool, drift: Bool) -> MenuBarStatus {
        MenuBarStatus(schemaVersion: 1, revision: "r", gateway: gateway, topology: "same_wifi_dhcp",
                      lanIp: "192.168.1.20", dhcp: "running", mihomo: "running", pfAnchor: "loaded",
                      forwarding: "enabled", clientCount: 2, drift: drift, doctorHealthy: true,
                      recoveryRequired: recovery, recoveryStage: recovery ? "gateway_active" : nil,
                      warnings: ["secret-value"], errorCode: nil)
    }
}

@MainActor
private final class RecordingMenuBarPresenter: MenuBarPresenting {
    var presentationCount = 0
    var stateChangeCount = 0
    func showPanel() { presentationCount += 1 }
    func applicationDidBecomeActive() { stateChangeCount += 1 }
}

private func panelPresentationAction(
    anchorVisible: Bool = true,
    popoverShown: Bool = true,
    panelWindowAvailable: Bool = true,
    popoverWindowWaitExpired: Bool = true
) -> MenuBarPanelPresentationAction {
    menuBarPanelPresentationAction(
        presentationPending: true,
        anchorVisible: anchorVisible,
        popoverShown: popoverShown,
        panelWindowAvailable: panelWindowAvailable,
        popoverWindowWaitExpired: popoverWindowWaitExpired
    )
}
