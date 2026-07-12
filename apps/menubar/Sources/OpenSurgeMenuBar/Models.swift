import Foundation

struct MenuBarStatus: Codable, Equatable {
    let schemaVersion: Int
    let revision: String
    let gateway: String
    let topology: String
    let lanIp: String
    let dhcp: String
    let mihomo: String
    let pfAnchor: String
    let forwarding: String
    let clientCount: Int
    let drift: Bool
    let doctorHealthy: Bool
    let recoveryRequired: Bool
    let recoveryStage: String?
    let warnings: [String]
    let errorCode: String?

    enum CodingKeys: String, CodingKey {
        case schemaVersion = "schema_version"
        case revision, gateway, topology
        case lanIp = "lan_ip"
        case dhcp, mihomo
        case pfAnchor = "pf_anchor"
        case forwarding
        case clientCount = "client_count"
        case drift
        case doctorHealthy = "doctor_healthy"
        case recoveryRequired = "recovery_required"
        case recoveryStage = "recovery_stage"
        case warnings
        case errorCode = "error_code"
    }
}

struct BootstrapResponse: Codable {
    let schemaVersion: Int
    let url: URL
    let expiresAt: Date

    enum CodingKeys: String, CodingKey {
        case schemaVersion = "schema_version"
        case url
        case expiresAt = "expires_at"
    }
}

struct EndpointDescriptor: Codable {
    let schemaVersion: Int
    let url: URL

    enum CodingKeys: String, CodingKey {
        case schemaVersion = "schema_version"
        case url
    }
}

enum IndicatorState: Equatable {
    case stopped, running, degraded, recovery, unreachable

    var systemImage: String {
        switch self {
        case .stopped: "network"
        case .running: "network.badge.shield.half.filled"
        case .degraded: "exclamationmark.circle"
        case .recovery: "exclamationmark.triangle.fill"
        case .unreachable: "network.slash"
        }
    }

    var accessibilityLabel: String {
        switch self {
        case .stopped: "OpenSurge 网关已停止"
        case .running: "OpenSurge 网关正在运行"
        case .degraded: "OpenSurge 网关运行异常"
        case .recovery: "OpenSurge 网络恢复尚未完成"
        case .unreachable: "无法连接 OpenSurge 控制服务"
        }
    }
}

extension MenuBarStatus {
    var topologyLabel: String {
        switch topology {
        case "same_wifi_dhcp": "同一 LAN DHCP 接管"
        case "same_lan": "同 LAN 手工网关"
        case "isolated_lan": "独立下游 LAN"
        default: topology
        }
    }

    var recoveryHasChangedNetwork: Bool {
        recoveryRequired && recoveryStage != "prepared"
    }

    var recoverySnapshotPrepared: Bool {
        recoveryRequired && recoveryStage == "prepared"
    }

    var indicator: IndicatorState {
        if recoveryHasChangedNetwork { return .recovery }
        if gateway == "degraded" || drift || !doctorHealthy { return .degraded }
        if gateway == "running" { return .running }
        return .stopped
    }

    var diagnosticSummary: String {
        [
            "OpenSurge for Mac",
            "Gateway: \(gateway)",
            "Topology: \(topologyLabel) [\(topology)]",
            "LAN IP: \(lanIp)",
            "DHCP/DNS: \(dhcp)",
            "mihomo: \(mihomo)",
            "PF: \(pfAnchor)",
            "Forwarding: \(forwarding)",
            "Clients: \(clientCount)",
            "Drift: \(drift)",
            "Recovery: \(recoveryRequired ? recoveryStage ?? "required" : "none")",
            "Error code: \(errorCode ?? "none")",
        ].joined(separator: "\n")
    }
}
