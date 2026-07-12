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

    func testActiveTakeoverUsesRunningIndicatorInsteadOfRecovery() {
        let status = MenuBarStatus(schemaVersion: 1, revision: "r", gateway: "running", topology: "same_wifi_dhcp",
                                   lanIp: "192.168.1.20", dhcp: "running", mihomo: "running", pfAnchor: "loaded",
                                   forwarding: "enabled", clientCount: 2, drift: false, doctorHealthy: true,
                                   recoveryRequired: true, recoveryStage: "gateway_active", warnings: [], errorCode: nil)
        XCTAssertTrue(status.takeoverActive)
        XCTAssertFalse(status.recoveryNeedsAttention)
        XCTAssertEqual(status.indicator, .running)
    }

    func testDiagnosticSummaryDoesNotContainWarnings() {
        let status = fixture(gateway: "running", recovery: false, drift: false)
        XCTAssertFalse(status.diagnosticSummary.contains("secret-value"))
    }

    func testSameWiFiTechnicalModeUsesSameLANProductLabel() {
        XCTAssertEqual(fixture(gateway: "stopped", recovery: false, drift: false).topologyLabel, "同一 LAN DHCP 接管")
    }

    private func fixture(gateway: String, recovery: Bool, drift: Bool) -> MenuBarStatus {
        MenuBarStatus(schemaVersion: 1, revision: "r", gateway: gateway, topology: "same_wifi_dhcp",
                      lanIp: "192.168.1.20", dhcp: "running", mihomo: "running", pfAnchor: "loaded",
                      forwarding: "enabled", clientCount: 2, drift: drift, doctorHealthy: true,
                      recoveryRequired: recovery, recoveryStage: recovery ? "gateway_active" : nil,
                      warnings: ["secret-value"], errorCode: nil)
    }
}
