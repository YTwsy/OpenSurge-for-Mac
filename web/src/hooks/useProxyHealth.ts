import { useCallback, useEffect, useMemo, useState } from 'react'
import { api } from '../api'
import type { ProxyHealthEntry, ProxyHealthSnapshot } from '../types'

export function useProxyHealth() {
  const [snapshot, setSnapshot] = useState<ProxyHealthSnapshot | null>(null)
  const [testing, setTesting] = useState<Set<string>>(new Set())
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    try {
      setSnapshot(await api.proxyHealth())
      setError('')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    }
  }, [])

  useEffect(() => { void refresh() }, [refresh])

  const test = useCallback(async (names: string[]) => {
    const unique = [...new Set(names.filter(Boolean))]
    if (!unique.length) return
    setTesting(current => new Set([...current, ...unique]))
    setError('')
    try {
      for (let offset = 0; offset < unique.length; offset += 120) {
        const response = await api.testProxyHealth(unique.slice(offset, offset + 120))
        const results = new Map(response.results.map(result => [result.name, result]))
        setSnapshot(current => current ? {
          ...current,
          test_url: response.test_url,
          proxies: current.proxies.map(proxy => {
            const result = results.get(proxy.name)
            return result ? { ...proxy, ...result } satisfies ProxyHealthEntry : proxy
          }),
        } : current)
      }
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setTesting(current => {
        const next = new Set(current)
        unique.forEach(name => next.delete(name))
        return next
      })
    }
  }, [refresh])

  const byName = useMemo(() => new Map(snapshot?.proxies.map(proxy => [proxy.name, proxy]) ?? []), [snapshot])
  return { snapshot, byName, testing, error, refresh, test }
}
