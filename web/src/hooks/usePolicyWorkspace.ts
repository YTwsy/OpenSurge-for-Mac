import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from '../api'
import type { PolicyWorkspaceRequest, PolicyWorkspaceSnapshot } from '../types'

function normalizeSnapshot(snapshot: PolicyWorkspaceSnapshot): PolicyWorkspaceSnapshot {
  return {
    ...snapshot,
    groups: (snapshot.groups ?? []).map(group => ({ ...group, options: group.options ?? [] })),
    health: { ...(snapshot.health ?? { schema_version: 1, test_url: '' }), proxies: snapshot.health?.proxies ?? [] },
  }
}

// A prepared core and the gateway core expose the same policy view. Keep that
// view together: a late read must not overwrite a newly acknowledged selection.
export function usePolicyWorkspace(refreshKey: string) {
  const [snapshot, setSnapshot] = useState<PolicyWorkspaceSnapshot | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [testing, setTesting] = useState<Set<string>>(new Set())
  const mounted = useRef(false)
  const readVersion = useRef(0)
  const activeReads = useRef(0)
  const pendingMutations = useRef(0)
  const refreshPending = useRef(false)
  const mutationQueue = useRef<Promise<unknown>>(Promise.resolve())

  const refresh = useCallback(async () => {
    if (pendingMutations.current) {
      refreshPending.current = true
      return
    }
    const version = ++readVersion.current
    activeReads.current += 1
    setLoading(true)
    try {
      const response = await api.policyWorkspace({ action: 'read' })
      if (mounted.current && version === readVersion.current) {
        setSnapshot(normalizeSnapshot(response))
        setError('')
      }
    } catch (cause) {
      if (mounted.current && version === readVersion.current) {
        setSnapshot(null)
        setError(cause instanceof Error ? cause.message : String(cause))
      }
    } finally {
      activeReads.current -= 1
      if (mounted.current && version === readVersion.current) setLoading(false)
    }
  }, [])

  useEffect(() => {
    mounted.current = true
    return () => { mounted.current = false; readVersion.current += 1 }
  }, [])

  useEffect(() => { void refresh() }, [refresh, refreshKey])

  useEffect(() => {
    const timer = window.setInterval(() => {
      if (!document.hidden && !activeReads.current && !pendingMutations.current) void refresh()
    }, 5000)
    return () => { window.clearInterval(timer) }
  }, [refresh])

  const mutate = useCallback((request: Exclude<PolicyWorkspaceRequest, { action: 'read' }>) => {
    pendingMutations.current += 1
    readVersion.current += 1
    const operation = mutationQueue.current.then(async () => {
      const response = await api.policyWorkspace(request)
      if (mounted.current) {
        setSnapshot(normalizeSnapshot(response))
        setError('')
        setLoading(false)
      }
      return response
    }).catch(cause => {
      if (mounted.current) {
        setSnapshot(null)
        setError(cause instanceof Error ? cause.message : String(cause))
      }
      throw cause
    }).finally(() => {
      pendingMutations.current -= 1
      if (mounted.current && !pendingMutations.current) {
        setLoading(false)
        if (refreshPending.current) {
          refreshPending.current = false
          void refresh()
        }
      }
    })
    mutationQueue.current = operation.catch(() => {})
    return operation
  }, [refresh])

  const select = useCallback((group: string, policy: string) => mutate({ action: 'select', group, policy }), [mutate])

  const test = useCallback(async (names: string[]) => {
    const unique = [...new Set(names.filter(Boolean))]
    if (!unique.length) return
    setTesting(current => new Set([...current, ...unique]))
    setError('')
    try {
      for (let offset = 0; offset < unique.length; offset += 120) {
        await mutate({ action: 'test', names: unique.slice(offset, offset + 120) })
      }
    } catch (cause) {
      if (mounted.current) setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      if (mounted.current) setTesting(current => {
        const next = new Set(current)
        unique.forEach(name => next.delete(name))
        return next
      })
    }
  }, [mutate])

  const byName = useMemo(() => new Map(snapshot?.health.proxies.map(proxy => [proxy.name, proxy]) ?? []), [snapshot])
  return { snapshot, byName, loading, error, testing, refresh, select, test }
}
