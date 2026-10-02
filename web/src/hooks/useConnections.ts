import { useEffect, useState } from 'react'
import { api } from '../api'
import { isWindowVisible } from '../visibility'
import type { ConnectionObservation } from '../types'

export function useConnections(paused: boolean, gateway?: string, revision?: string) {
  const [snapshot, setSnapshot] = useState<ConnectionObservation | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
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
          if (active) { setSnapshot(value); setError('') }
        } catch (cause) {
          if (active) setError(cause instanceof Error ? cause.message : String(cause))
        } finally { inFlight = false }
      }
      if (active) timer = window.setTimeout(() => void poll(), 2000)
    }
    const onVisibility = () => { if (isWindowVisible()) void poll() }
    document.addEventListener('visibilitychange', onVisibility)
    void poll()
    return () => { active = false; controller.abort(); window.clearTimeout(timer); document.removeEventListener('visibilitychange', onVisibility) }
  }, [paused, gateway, revision])

  return { snapshot, error }
}
