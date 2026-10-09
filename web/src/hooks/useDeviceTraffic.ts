import { createContext, createElement, useContext, useEffect, useState, type ReactNode } from 'react'
import { api } from '../api'
import { watchVisibleRefresh } from '../visibility'
import type { DeviceTraffic, TrafficHistoryPoint } from '../types'

const refreshIntervalMs = 2_000
const historyLimit = 30
const historyWindowMs = 60_000
export const gatewayLocalDeviceKey = 'gateway-local'

type TrafficSnapshot = { traffic: DeviceTraffic | null; history: TrafficHistoryPoint[]; error: string }
const emptySnapshot: TrafficSnapshot = { traffic: null, history: [], error: '' }
const DeviceTrafficContext = createContext<TrafficSnapshot>(emptySnapshot)

// Own sampling above the route boundary: navigation must not discard the chart's
// history/scale, and pages that do not consume this context do not render on ticks.
export function DeviceTrafficProvider({ gateway, children }: { gateway?: string; children: ReactNode }) {
  const [snapshot, setSnapshot] = useState<TrafficSnapshot>(emptySnapshot)
  const lifecycle = gateway === 'degraded' ? 'running' : gateway

  useEffect(() => {
    let active = true
    setSnapshot(emptySnapshot)
    if (!lifecycle) return

    const refresh = async () => {
      try {
        const next = await api.deviceTraffic()
        if (!active) return
        setSnapshot(current => {
          const latest = current.history.at(-1)?.sampled_at
          if (latest && Date.parse(next.sampled_at) < Date.parse(latest)) return current
          return { traffic: next, history: appendTrafficPoint(current.history, next), error: '' }
        })
      } catch (cause) {
        if (active) setSnapshot(current => ({ ...current, error: cause instanceof Error ? cause.message : String(cause) }))
      }
    }

    const stopRefresh = watchVisibleRefresh(refresh, refreshIntervalMs)
    return () => {
      active = false
      stopRefresh()
    }
  }, [lifecycle])

  return createElement(DeviceTrafficContext.Provider, { value: snapshot }, children)
}

export function useDeviceTraffic() {
  return useContext(DeviceTrafficContext)
}

export function appendTrafficPoint(history: TrafficHistoryPoint[], traffic: DeviceTraffic) {
  const timestamp = Date.parse(traffic.sampled_at)
  const latest = history.at(-1)?.sampled_at
  if (!Number.isFinite(timestamp) || (latest && timestamp <= Date.parse(latest))) return history
  const devices = Object.fromEntries(traffic.devices.map(device => [deviceKey(device.mac, device.ip), {
    upload: device.upload_rate ?? 0,
    download: device.download_rate ?? 0,
  }]))
  devices[gatewayLocalDeviceKey] = {
    upload: traffic.gateway_local.upload_rate ?? 0,
    download: traffic.gateway_local.download_rate ?? 0,
  }
  const point: TrafficHistoryPoint = {
    sampled_at: traffic.sampled_at,
    upload: traffic.gateway_rates?.upload ?? traffic.totals.upload_rate ?? 0,
    download: traffic.gateway_rates?.download ?? traffic.totals.download_rate ?? 0,
    devices,
  }
  return [...history.filter(sample => timestamp - Date.parse(sample.sampled_at) < historyWindowMs), point].slice(-historyLimit)
}

export function deviceKey(mac: string, ip: string) {
  return `${mac.trim().toLowerCase()}-${ip.trim()}`
}
