// swift-tools-version: 5.7
import PackageDescription

let package = Package(
    name: "WiseInvest",
    platforms: [
        .iOS(.v15)
    ],
    products: [
        .library(
            name: "WiseInvest",
            targets: ["WiseInvest"]),
    ],
    dependencies: [],
    targets: [
        .target(
            name: "WiseInvest",
            dependencies: [],
            path: "WiseInvest"),
    ]
)
