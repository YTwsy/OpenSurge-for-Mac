import Foundation

struct MenuBarStatus: Codable, Equatable {
    let schemaVersion: Int
    let revision: String
    let gateway: String
    let topology: String
    let lanIp: String
    let dhcp: String
    let mihomo: String
    let tun: String?
    let tunInterface: String?
    let tunError: String?
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
        case dhcp, mihomo, tun
        case tunInterface = "tun_interface"
        case tunError = "tun_error"
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

    init(
        schemaVersion: Int,
        revision: String,
        gateway: String,
        topology: String,
        lanIp: String,
        dhcp: String,
        mihomo: String,
        tun: String? = nil,
        tunInterface: String? = nil,
        tunError: String? = nil,
        pfAnchor: String,
        forwarding: String,
        clientCount: Int,
        drift: Bool,
        doctorHealthy: Bool,
        recoveryRequired: Bool,
        recoveryStage: String?,
        warnings: [String],
        errorCode: String?
    ) {
        self.schemaVersion = schemaVersion
        self.revision = revision
        self.gateway = gateway
        self.topology = topology
        self.lanIp = lanIp
        self.dhcp = dhcp
        self.mihomo = mihomo
        self.tun = tun
        self.tunInterface = tunInterface
        self.tunError = tunError
        self.pfAnchor = pfAnchor
        self.forwarding = forwarding
        self.clientCount = clientCount
        self.drift = drift
        self.doctorHealthy = doctorHealthy
        self.recoveryRequired = recoveryRequired
        self.recoveryStage = recoveryStage
        self.warnings = warnings
        self.errorCode = errorCode
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
    case connecting, stopped, running, degraded, recovery, unreachable

    var usesBrandMenuBarIcon: Bool {
        self == .connecting || self == .stopped || self == .running || self == .unreachable
    }

    var menuBarIconOpacity: Double {
        switch self {
        case .connecting: 0.75
        case .stopped: 0.55
        case .unreachable: 0.35
        default: 1
        }
    }

    var systemImage: String {
        switch self {
        case .connecting: "network"
        case .stopped: "network"
        case .running: "network.badge.shield.half.filled"
        case .degraded: "exclamationmark.circle"
        case .recovery: "exclamationmark.triangle.fill"
        case .unreachable: "network.slash"
        }
    }

    var accessibilityLabel: String {
        switch self {
        case .connecting: "正在连接 OpenSurge 控制服务"
        case .stopped: "OpenSurge 网关已停止"
        case .running: "OpenSurge 网关正在运行"
        case .degraded: "OpenSurge 网关运行异常"
        case .recovery: "OpenSurge 网络恢复尚未完成"
        case .unreachable: "无法连接 OpenSurge 控制服务"
        }
    }
}

func menuBarIndicator(status: MenuBarStatus?, hasError: Bool) -> IndicatorState {
    if let status { return status.indicator }
    return hasError ? .unreachable : .connecting
}

extension MenuBarStatus {
    var gatewayServicesActive: Bool {
        gateway == "running" || gateway == "degraded" || dhcp == "running" || mihomo == "running" || pfAnchor == "loaded"
    }

    var canQuitOpenSurge: Bool {
        gateway == "stopped" && !gatewayServicesActive && !recoveryNeedsAttention
    }

    var canUninstall: Bool {
        gateway == "stopped"
    }

    var topologyLabel: String {
        switch topology {
        case "same_wifi_dhcp": "局域网 DHCP 接管"
        case "same_lan": "旁路由模式"
        case "isolated_lan": "独立下游 LAN"
        default: topology
        }
    }

    var recoveryNeedsAttention: Bool {
        guard recoveryRequired else { return false }
        guard let stage = recoveryStage else { return true }
        return !["prepared", "gateway_active", "client_validated", "client_validation_skipped"].contains(stage)
    }

    var takeoverActive: Bool {
        guard recoveryRequired, let stage = recoveryStage else { return false }
        return ["gateway_active", "client_validated", "client_validation_skipped"].contains(stage)
    }

    var recoverySnapshotPrepared: Bool {
        recoveryRequired && recoveryStage == "prepared"
    }

    var indicator: IndicatorState {
        if recoveryNeedsAttention { return .recovery }
        // A stopped gateway can legitimately fail runtime-oriented doctor
        // checks and can have unapplied desired config. Neither means a
        // gateway that is not running has suffered a runtime failure.
        if gateway == "stopped" { return .stopped }
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
            "TUN: \(tun ?? "unknown")\(tunInterface.map { " [\($0)]" } ?? "")",
            "PF: \(pfAnchor)",
            "Forwarding: \(forwarding)",
            "Clients: \(clientCount)",
            "Drift: \(drift)",
            "Recovery: \(recoveryRequired ? recoveryStage ?? "required" : "none")",
            "Error code: \(errorCode ?? "none")",
        ].joined(separator: "\n")
    }
}

func menuBarQuitWarning(for status: MenuBarStatus?) -> String {
    guard let status else {
        return "退出只会关闭菜单栏图标；OpenSurge 后台控制服务仍会继续运行。当前无法确认网关服务状态，请先在 Web GUI 或活动监视器中检查。"
    }
    if status.gatewayServicesActive {
        let recovery = status.recoveryRequired ? " 当前网络状态机尚未结束，退出也不会完成网络恢复。" : ""
        return "退出只会关闭菜单栏图标；正在运行的 DHCP/DNS、mihomo、PF/转发和后台控制服务都不会停止。请先在 Web GUI 中停止网关（如需要）。" + recovery
    }
    return "退出只会关闭菜单栏图标；网关当前未运行，但 OpenSurge 后台控制服务仍会继续运行。"
}

func openSurgeQuitWarning(for status: MenuBarStatus?) -> String {
    guard let status else {
        return "当前无法确认网关服务状态。请先重新连接后台服务，确认网关与网络恢复状态后再退出 OpenSurge。"
    }
    guard status.canQuitOpenSurge else {
        if status.recoveryNeedsAttention {
            return "网络恢复尚未完成。请先在网络设置中完成恢复，再退出 OpenSurge。"
        }
        return "网关数据面仍在运行。请先在网络设置中停止网关，确认 DHCP/DNS、mihomo 与 PF 已停止。"
    }
    return "网关数据面已经停止。此操作会退出菜单栏 App 和用户级 Control Service；由系统 launchd 托管的 root Helper 仍保持空闲加载，下次打开 OpenSurge 不需要再次授权。"
}

func uninstallWarning(for status: MenuBarStatus?) -> String {
    guard let status else {
        return "当前无法确认网关状态。请先重新连接后台服务，再卸载 OpenSurge。"
    }
    guard status.canUninstall else {
        return "网关仍在运行。请先在网络设置中停止网关，再卸载 OpenSurge。"
    }
    return "网关已经停止。卸载不会修改当前系统的 IPv4 forwarding 状态。"
}
