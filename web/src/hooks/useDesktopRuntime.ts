import { useCallback, useEffect, useRef, useState } from 'react'
import { desktopAction } from '../desktop'
import type { SleepPreventionStatus } from '../types'
import type { TraySnapshot } from '../tray/types'
import { watchVisibleRefresh } from '../visibility'

// Settings reads the same native status and capabilities as the menu bar.
// Gateway lifecycle eligibility remains in the host, including its fresh checks
// immediately before and after a native confirmation.
export function useDesktopRuntime() {
 const [snapshot, setSnapshot] = useState<TraySnapshot | null>(null)
 const [busy, setBusy] = useState(false)
 const [error, setError] = useState('')
 const active = useRef(true)
 const pending = useRef(false)
 const generation = useRef(0)
 const sequence = useRef(0)
 const refresh = useCallback(async () => {
  const requested = generation.current
  try {
   const next = await desktopAction<TraySnapshot>('menubar-status', { refresh: true })
   if (!active.current || next.sequence < sequence.current) return
   sequence.current = next.sequence
   setSnapshot(current => next.status && current?.status && (pending.current || requested !== generation.current)
    ? { ...next, status: { ...next.status, sleep_prevention: current.status.sleep_prevention } }
    : next)
  } catch {
   if (active.current) setSnapshot(current => ({ ...current, status: null, indicator: 'unreachable', sequence: sequence.current, can_quit: false, can_uninstall: false }))
  }
 }, [])
 useEffect(() => {
  active.current = true
  const stop = watchVisibleRefresh(refresh, 3000)
  return () => { active.current = false; stop() }
 }, [refresh])
 const perform = async (action: () => Promise<unknown>) => {
  if (pending.current) return
  pending.current = true; generation.current++; setBusy(true); setError('')
  try { await action() }
  catch (cause) { if (active.current) setError(cause instanceof Error ? cause.message : String(cause)) }
  finally {
   pending.current = false; generation.current++
   if (active.current) { setBusy(false); void refresh() }
  }
 }
 const changeSleep = (enabled: boolean) => perform(async () => {
  const sleep = await desktopAction<SleepPreventionStatus>('sleep-prevention', { enabled })
  if (active.current) setSnapshot(current => current?.status ? { ...current, status: { ...current.status, sleep_prevention: sleep } } : current)
 })
 const quit = (full: boolean) => perform(() => desktopAction('quit', { full }))
 return { snapshot, busy, error, changeSleep, quit }
}
