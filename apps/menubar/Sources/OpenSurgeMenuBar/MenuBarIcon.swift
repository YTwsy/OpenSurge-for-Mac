import AppKit
import SwiftUI

@MainActor
private enum OpenSurgeAppIconAsset {
    static let image: NSImage = {
        if let url = Bundle.main.url(forResource: "OpenSurgeAppIcon", withExtension: "icns"),
           let image = NSImage(contentsOf: url) {
            return image
        }

        return NSImage(systemSymbolName: "network", accessibilityDescription: nil) ?? NSImage(size: NSSize(width: 34, height: 34))
    }()
}

@MainActor
private enum OpenSurgeMenuBarIconAsset {
    static let image: NSImage = {
        if let url = Bundle.main.url(forResource: "OpenSurgeMenuBarIcon", withExtension: "png"),
           let image = NSImage(contentsOf: url) {
            image.isTemplate = true
            image.size = NSSize(width: 18, height: 18)
            return image
        }

        return NSImage(systemSymbolName: "network", accessibilityDescription: nil) ?? NSImage(size: NSSize(width: 18, height: 18))
    }()
}

struct OpenSurgeAppIconView: View {
    var body: some View {
        Image(nsImage: OpenSurgeAppIconAsset.image)
            .resizable()
            .interpolation(.high)
            .frame(width: 34, height: 34)
    }
}

struct OpenSurgeMenuBarLabel: View {
    let indicator: IndicatorState

    @ViewBuilder
    var body: some View {
        if indicator.usesBrandMenuBarIcon {
            Image(nsImage: OpenSurgeMenuBarIconAsset.image)
                .renderingMode(.template)
                .resizable()
                .interpolation(.high)
                .frame(width: 18, height: 18)
                .foregroundStyle(.primary)
                .opacity(indicator.menuBarIconOpacity)
        } else {
            Image(systemName: indicator.systemImage)
                .symbolRenderingMode(.monochrome)
        }
    }
}
