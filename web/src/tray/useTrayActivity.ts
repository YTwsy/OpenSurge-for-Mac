import { useEffect, useState } from 'react'
import { api } from '../api'
import { connectionOwnerKey } from '../connections'
import { desktopAction } from '../desktop'
import type { DeviceTraffic, LocalRouting } from '../types'
import { watchVisibleRefresh } from '../visibility'

export type TrayTrafficPoint = { time: number; upload: number; download: number }
export type TrayTrafficSnapshot = { traffic: DeviceTraffic | null; history: TrayTrafficPoint[] | null; failed: boolean }
type Activity = { traffic: DeviceTraffic | null; routing: LocalRouting | null; history: TrayTrafficPoint[]; deviceKeys: string[]; trafficFailed: boolean; routingFailed: boolean }
const empty: Activity = { traffic: null, routing: null, history: [], deviceKeys: [], trafficFailed: false, routingFailed: false }

// Native code keeps a bounded observation cache while WebViews sleep. The popup
// only reads that cache while visible; slow routing reads cannot stall the chart.
export function useTrayActivity(gateway?: string) {
  const [activity, setActivity] = useState<Activity>(empty)
  const running = gateway === 'running' || gateway === 'degraded'
  useEffect(() => {
    setActivity(empty)
    if (!running) return
    let active = true
    let rankedAt = 0
    const stopTraffic = watchVisibleRefresh(async () => {
      let snapshot: TrayTrafficSnapshot
      try { snapshot = await desktopAction<TrayTrafficSnapshot>('tray-activity') }
      catch { snapshot = { traffic: null, history: [], failed: true } }
      if (!active) return
      const next = snapshot.traffic
      const sampledAt = next ? Date.parse(next.sampled_at) : NaN
      const now = Number.isFinite(sampledAt) ? sampledAt : Date.now()
      const rerank = now - rankedAt >= 10_000 || !rankedAt
      if (rerank) rankedAt = now
      setActivity(current => {
        const candidates = next?.devices.filter(device => device.active_connections > 0) ?? []
        const ranked = [...candidates].sort((a, b) => (b.download_rate + b.upload_rate) - (a.download_rate + a.upload_rate) || connectionOwnerKey(a).localeCompare(connectionOwnerKey(b))).map(connectionOwnerKey)
        const retained = current.deviceKeys.filter(key => ranked.includes(key))
        const deviceKeys = (rerank ? ranked : [...retained, ...ranked.filter(key => !retained.includes(key))]).slice(0, 2)
        return { ...current, traffic: next, history: snapshot.history ?? [], deviceKeys, trafficFailed: snapshot.failed }
      })
    }, 1_000)
    const stopRouting = watchVisibleRefresh(async () => {
      try {
        const routing = await api.localRouting()
        if (active) setActivity(current => ({ ...current, routing, routingFailed: false }))
      } catch {
        if (active) setActivity(current => ({ ...current, routing: null, routingFailed: true }))
      }
    }, 2_000)
    return () => { active = false; stopTraffic(); stopRouting() }
  }, [running])
  return running ? activity : empty
}
