import XCTest
@testable import OpenSurgeMenuBar

final class LocalizationTests: XCTestCase {
    func testSystemLanguageResolutionUsesFirstPreference() {
        XCTAssertEqual(AppLanguageResolver.resolve(.system, preferredLanguages: ["zh-Hant-HK", "en-US"]), .simplifiedChinese)
        XCTAssertEqual(AppLanguageResolver.resolve(.system, preferredLanguages: ["en-GB", "zh-Hans"]), .english)
        XCTAssertEqual(AppLanguageResolver.resolve(.system, preferredLanguages: ["ja-JP"]), .english)
    }

    func testExplicitLanguageOverridesSystemPreference() {
        XCTAssertEqual(AppLanguageResolver.resolve(.simplifiedChinese, preferredLanguages: ["en-US"]), .simplifiedChinese)
        XCTAssertEqual(AppLanguageResolver.resolve(.english, preferredLanguages: ["zh-Hans"]), .english)
    }

    func testMenuBarStatusDecodesSharedLanguagePreference() throws {
        let data = Data(#"{"schema_version":1,"revision":"r","gateway":"stopped","topology":"same_wifi_dhcp","lan_ip":"192.168.1.20","dhcp":"stopped","mihomo":"stopped","pf_anchor":"unloaded","forwarding":"disabled","client_count":0,"drift":false,"doctor_healthy":true,"recovery_required":false,"warnings":[],"ui_preferences":{"schema_version":1,"language":"en"}}"#.utf8)
        let status = try JSONDecoder().decode(MenuBarStatus.self, from: data)
        XCTAssertEqual(status.uiPreferences?.language, .english)
    }
}
