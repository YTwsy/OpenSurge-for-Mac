import { useEffect, useState } from 'react'
import { api } from '../api'
import { connectionOwnerKey } from '../connections'
import type { DeviceTraffic, LocalRouting } from '../types'
import { watchVisibleRefresh } from '../visibility'

export type TrayTrafficPoint = { time: number; upload: number; download: number }
type Activity = { traffic: DeviceTraffic | null; routing: LocalRouting | null; history: TrayTrafficPoint[]; deviceKeys: string[]; trafficFailed: boolean; routingFailed: boolean }
const empty: Activity = { traffic: null, routing: null, history: [], deviceKeys: [], trafficFailed: false, routingFailed: false }

// This is a view of the existing Control API, never a second traffic sampler.
// Stop reading when the gateway or popup is unavailable, and don't join a chart
// across a hidden-window gap or turn a failed read into a plausible zero rate.
export function useTrayActivity(gateway?: string) {
  const [activity, setActivity] = useState<Activity>(empty)
  const running = gateway === 'running' || gateway === 'degraded'
  useEffect(() => {
    setActivity(empty)
    if (!running) return
    let active = true
    let rankedAt = 0
    const stop = watchVisibleRefresh(async () => {
      const [traffic, routing] = await Promise.allSettled([api.deviceTraffic(), api.localRouting()])
      if (!active) return
      const next = traffic.status === 'fulfilled' && !traffic.value.connection_error ? traffic.value : null
      const sampledAt = next ? Date.parse(next.sampled_at) : NaN
      const now = Number.isFinite(sampledAt) ? sampledAt : Date.now()
      const rerank = now - rankedAt >= 10_000 || !rankedAt
      if (rerank) rankedAt = now
      setActivity(current => {
        const candidates = next?.devices.filter(device => device.active_connections > 0) ?? []
        const ranked = [...candidates].sort((a, b) => (b.download_rate + b.upload_rate) - (a.download_rate + a.upload_rate) || connectionOwnerKey(a).localeCompare(connectionOwnerKey(b))).map(connectionOwnerKey)
        const retained = current.deviceKeys.filter(key => ranked.includes(key))
        const deviceKeys = (rerank ? ranked : [...retained, ...ranked.filter(key => !retained.includes(key))]).slice(0, 2)
        const previous = current.history.at(-1)
        let history = next ? current.history.filter(point => now - point.time < 60_000 && now >= point.time) : []
        if (!previous || now - previous.time > 6_000 || now < previous.time) history = []
        if (next && previous?.time !== now) history = [...history, { time: now, ...next.gateway_rates }].slice(-30)
        return { traffic: next, history, deviceKeys, routing: routing.status === 'fulfilled' ? routing.value : null, trafficFailed: !next, routingFailed: routing.status === 'rejected' }
      })
    }, 2_000)
    return () => { active = false; stop() }
  }, [running])
  return running ? activity : empty
}
