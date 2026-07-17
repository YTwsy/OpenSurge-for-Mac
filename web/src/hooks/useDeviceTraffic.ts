import { useEffect, useState } from 'react'
import { api } from '../api'
import type { DeviceTraffic, TrafficHistoryPoint } from '../types'

const refreshIntervalMs = 2_000
const historyLimit = 30

export function useDeviceTraffic(gateway?: string) {
  const [traffic, setTraffic] = useState<DeviceTraffic | null>(null)
  const [history, setHistory] = useState<TrafficHistoryPoint[]>([])
  const [error, setError] = useState('')

  useEffect(() => {
    let active = true
    setTraffic(null)
    setHistory([])
    setError('')

    const refresh = async () => {
      try {
        const next = await api.deviceTraffic()
        if (!active) return
        setTraffic(next)
        setHistory(current => appendTrafficPoint(current, next))
        setError('')
      } catch (cause) {
        if (active) setError(cause instanceof Error ? cause.message : String(cause))
      }
    }

    void refresh()
    const timer = window.setInterval(() => void refresh(), refreshIntervalMs)
    return () => {
      active = false
      window.clearInterval(timer)
    }
  }, [gateway])

  return { traffic, history, error }
}

function appendTrafficPoint(history: TrafficHistoryPoint[], traffic: DeviceTraffic) {
  if (history.at(-1)?.sampled_at === traffic.sampled_at) return history
  const devices = Object.fromEntries(traffic.devices.map(device => [deviceKey(device.mac, device.ip), {
    upload: device.upload_rate ?? 0,
    download: device.download_rate ?? 0,
  }]))
  const point: TrafficHistoryPoint = {
    sampled_at: traffic.sampled_at,
    upload: traffic.gateway_rates?.upload ?? traffic.totals.upload_rate ?? 0,
    download: traffic.gateway_rates?.download ?? traffic.totals.download_rate ?? 0,
    devices,
  }
  return [...history, point].slice(-historyLimit)
}

export function deviceKey(mac: string, ip: string) {
  return `${mac.trim().toLowerCase()}-${ip.trim()}`
}
