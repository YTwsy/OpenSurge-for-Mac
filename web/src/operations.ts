import type { Operation } from './types'

export type TrackedOperation = Operation & { connection: 'connected' | 'reconnecting' | 'unknown'; dismissed?: boolean }
export const operationStatusUnknownMessage = '操作结果尚未确认，请勿重复执行；可在全局进度卡重新查询状态。'

let snapshot: TrackedOperation[] = []
const listeners = new Set<() => void>()

export const getOperations = () => snapshot
export const getOperation = (id: string) => snapshot.find(operation => operation.id === id)
export function subscribeOperations(listener: () => void) {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}

function publish(next: TrackedOperation[]) {
  snapshot = next
  listeners.forEach(listener => listener())
}

export function recordOperation(operation: Operation) {
  const previous = getOperation(operation.id)
  // A late poll must never put a completed synchronous request back in flight.
  if (previous && previous.state !== 'running' && operation.state === 'running') return
  if (previous?.updated_at && operation.updated_at && Date.parse(operation.updated_at) < Date.parse(previous.updated_at) && previous.state === 'running' && operation.state === 'running') return
  const updatedAt = operation.updated_at || (operation.state !== 'running' && previous?.state === 'running' ? new Date().toISOString() : previous?.updated_at)
  const next: TrackedOperation = { ...previous, ...operation, updated_at: updatedAt, connection: 'connected' }
  publish([...snapshot.filter(item => item.id !== operation.id), next].slice(-20))
}

export function markOperationConnection(id: string, connection: TrackedOperation['connection']) {
  publish(snapshot.map(operation => operation.id === id && operation.state === 'running' ? { ...operation, connection } : operation))
}

export function dismissOperation(id: string) {
  publish(snapshot.map(operation => operation.id === id ? { ...operation, dismissed: true } : operation))
}

export function clearOperations() {
  publish([])
}
