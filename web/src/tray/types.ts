import type { SleepPreventionStatus, UIPreferences } from '../types'

export type MenuBarStatus = {
 schema_version: number
 revision: string
 gateway: string
 topology: string
 lan_ip: string
 dhcp: string
 mihomo: string
 tun: string
 tun_interface?: string
 pf_anchor: string
 forwarding: string
 ipv4_takeover: string
 ipv6_takeover: string
 client_count: number
 drift: boolean
 doctor_healthy: boolean
 recovery_required: boolean
 recovery_stage?: string
 error_code?: string
 warnings: string[]
 sleep_prevention: SleepPreventionStatus
 ui_preferences: UIPreferences
}
export type TraySnapshot = {
 status: MenuBarStatus | null
 indicator: 'connecting' | 'stopped' | 'running' | 'degraded' | 'recovery' | 'unreachable'
 sequence: number
 can_quit: boolean
}
