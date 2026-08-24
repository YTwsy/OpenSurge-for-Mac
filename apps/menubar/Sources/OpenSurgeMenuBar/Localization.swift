import Foundation

enum RequestedAppLanguage: String, Codable, Equatable {
    case system
    case simplifiedChinese = "zh-Hans"
    case english = "en"
}

enum ResolvedAppLanguage: String, Equatable {
    case simplifiedChinese = "zh-Hans"
    case english = "en"
}

enum AppLanguageResolver {
    static func resolve(
        _ requested: RequestedAppLanguage,
        preferredLanguages: [String] = Locale.preferredLanguages
    ) -> ResolvedAppLanguage {
        switch requested {
        case .simplifiedChinese:
            return .simplifiedChinese
        case .english:
            return .english
        case .system:
            let first = preferredLanguages.first?.lowercased() ?? ""
            return first == "zh" || first.hasPrefix("zh-")
                ? .simplifiedChinese
                : .english
        }
    }
}

enum L10n {
    nonisolated(unsafe) private(set) static var requestedLanguage: RequestedAppLanguage = .system

    static var resolvedLanguage: ResolvedAppLanguage {
        AppLanguageResolver.resolve(requestedLanguage)
    }

    static func activate(_ requested: RequestedAppLanguage) {
        requestedLanguage = requested
    }

    static func text(_ source: String) -> String {
        guard resolvedLanguage == .english,
              let path = Bundle.main.path(forResource: "en", ofType: "lproj"),
              let bundle = Bundle(path: path) else {
            return source
        }
        return bundle.localizedString(forKey: source, value: source, table: nil)
    }

    static func format(_ source: String, _ arguments: CVarArg...) -> String {
        String(format: text(source), locale: locale, arguments: arguments)
    }

    static var locale: Locale {
        Locale(identifier: resolvedLanguage == .simplifiedChinese ? "zh_CN" : "en_US")
    }
}
