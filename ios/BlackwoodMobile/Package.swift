// swift-tools-version: 6.0

import PackageDescription

let package = Package(
    name: "BlackwoodMobileCore",
    platforms: [
        .macOS(.v14),
    ],
    products: [
        .library(
            name: "BlackwoodMobileCore",
            targets: ["BlackwoodMobileCore"]
        ),
    ],
    targets: [
        .target(
            name: "BlackwoodMobileCore",
            path: "Sources/BlackwoodMobileCore"
        ),
        .testTarget(
            name: "BlackwoodMobileCoreTests",
            dependencies: ["BlackwoodMobileCore"],
            path: "Tests/BlackwoodMobileCoreTests"
        ),
    ]
)
