import { useEffect, useState } from 'react'
import { api } from '../api'
import { isWindowVisible } from '../visibility'
import type { ConnectionObservation } from '../types'

export function useConnections(paused: boolean, gateway?: string, revision?: string) {
  const lifecycle = gateway === 'degraded' ? 'running' : gateway
  const [observation, setObservation] = useState<{ lifecycle?: string; snapshot: ConnectionObservation | null; error: string }>({ lifecycle, snapshot: null, error: '' })

  useEffect(() => {
    setObservation(current => current.lifecycle === lifecycle ? current : { lifecycle, snapshot: null, error: '' })
    if (paused) return
    let active = true
    let timer = 0
    let inFlight = false
    const controller = new AbortController()
    const poll = async () => {
      if (!active || inFlight) return
      window.clearTimeout(timer)
      if (isWindowVisible()) {
        inFlight = true
        try {
          const value = await api.connections(controller.signal)
          if (active) setObservation({ lifecycle, snapshot: value, error: '' })
        } catch (cause) {
          if (active) setObservation(current => ({ lifecycle, snapshot: current.lifecycle === lifecycle ? current.snapshot : null, error: cause instanceof Error ? cause.message : String(cause) }))
        } finally { inFlight = false }
      }
      if (active) timer = window.setTimeout(() => void poll(), 2000)
    }
    const onVisibility = () => { if (isWindowVisible()) void poll() }
    document.addEventListener('visibilitychange', onVisibility)
    void poll()
    return () => { active = false; controller.abort(); window.clearTimeout(timer); document.removeEventListener('visibilitychange', onVisibility) }
  }, [paused, lifecycle, revision])

  // A stopped inventory and a running sample belong to different lifecycles.
  // Never briefly present connections or errors from the previous run.
  return observation.lifecycle === lifecycle ? { snapshot: observation.snapshot, error: observation.error } : { snapshot: null, error: '' }
}
