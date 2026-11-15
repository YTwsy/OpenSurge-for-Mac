// swift-tools-version: 5.10
import PackageDescription

let package = Package(
    name: "OpenSurgeMenuBar",
    // macOS 12 (Monterey) keeps intel Macs supported; v13 would drop them.
    platforms: [.macOS(.v12)],
    products: [.executable(name: "OpenSurgeMenuBar", targets: ["OpenSurgeMenuBar"])],
    targets: [
        .executableTarget(name: "OpenSurgeMenuBar"),
        .testTarget(name: "OpenSurgeMenuBarTests", dependencies: ["OpenSurgeMenuBar"]),
    ]
)
