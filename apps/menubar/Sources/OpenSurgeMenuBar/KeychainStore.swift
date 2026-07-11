import Foundation
import Security

enum KeychainStore {
    private static let service = "com.opensurge.control"
    private static let account = "menubar-client-token"

    static func token() -> String? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne,
        ]
        var value: CFTypeRef?
        guard SecItemCopyMatching(query as CFDictionary, &value) == errSecSuccess,
              let data = value as? Data else { return nil }
        return String(data: data, encoding: .utf8)
    }

    static func save(token: String) throws {
        guard let data = token.data(using: .utf8) else { throw ControlAPIError.tokenUnavailable }
        let identity: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ]
        let update = [kSecValueData as String: data]
        let status = SecItemUpdate(identity as CFDictionary, update as CFDictionary)
        if status == errSecItemNotFound {
            var create = identity
            create[kSecValueData as String] = data
            let created = SecItemAdd(create as CFDictionary, nil)
            guard created == errSecSuccess else { throw ControlAPIError.keychain(created) }
        } else if status != errSecSuccess {
            throw ControlAPIError.keychain(status)
        }
    }
}
