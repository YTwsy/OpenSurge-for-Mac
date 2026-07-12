import XCTest
@testable import OpenSurgeMenuBar

final class ModelsTests: XCTestCase {
    func testRecoveryHasHighestIndicatorPriority() {
        let status = fixture(gateway: "running", recovery: true, drift: true)
        XCTAssertEqual(status.indicator, .recovery)
    }

    func testPreparedRecoveryCardDoesNotImplyNetworkHasChanged() {
        let status = MenuBarStatus(schemaVersion: 1, revision: "r", gateway: "stopped", topology: "same_wifi_dhcp",
                                   lanIp: "192.168.1.20", dhcp: "stopped", mihomo: "stopped", pfAnchor: "unloaded",
                                   forwarding: "disabled", clientCount: 0, drift: false, doctorHealthy: true,
                                   recoveryRequired: true, recoveryStage: "prepared", warnings: [], errorCode: nil)
        XCTAssertTrue(status.recoverySnapshotPrepared)
        XCTAssertFalse(status.recoveryHasChangedNetwork)
        XCTAssertEqual(status.indicator, .stopped)
    }

    func testDriftIsDegraded() {
        XCTAssertEqual(fixture(gateway: "running", recovery: false, drift: true).indicator, .degraded)
    }

    func testDiagnosticSummaryDoesNotContainWarnings() {
        let status = fixture(gateway: "running", recovery: false, drift: false)
        XCTAssertFalse(status.diagnosticSummary.contains("secret-value"))
    }

    private func fixture(gateway: String, recovery: Bool, drift: Bool) -> MenuBarStatus {
        MenuBarStatus(schemaVersion: 1, revision: "r", gateway: gateway, topology: "same_wifi_dhcp",
                      lanIp: "192.168.1.20", dhcp: "running", mihomo: "running", pfAnchor: "loaded",
                      forwarding: "enabled", clientCount: 2, drift: drift, doctorHealthy: true,
                      recoveryRequired: recovery, recoveryStage: recovery ? "gateway_active" : nil,
                      warnings: ["secret-value"], errorCode: nil)
    }
}
