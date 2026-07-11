// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "OpenSurgeMenuBar",
    platforms: [.macOS(.v13)],
    products: [.executable(name: "OpenSurgeMenuBar", targets: ["OpenSurgeMenuBar"])],
    targets: [
        .executableTarget(name: "OpenSurgeMenuBar"),
        .testTarget(name: "OpenSurgeMenuBarTests", dependencies: ["OpenSurgeMenuBar"]),
    ]
)
