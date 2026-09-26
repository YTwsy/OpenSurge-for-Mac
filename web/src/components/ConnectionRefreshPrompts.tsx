import { useEffect, useState } from 'react'
import { api } from '../api'
import { t } from '../i18n'

export type ConnectionRefreshSuggestion = {
  key: string
  subject: string
  selection: string
} & ({
  scope: 'gateway_local'
} | {
  scope: 'device'
  deviceID: string
} | {
  scope: 'policy_group'
  group: string
})

export type ConnectionRefreshSuggestionItem = ConnectionRefreshSuggestion & { id: number }

export function queueConnectionRefreshSuggestion(current: ConnectionRefreshSuggestionItem[], suggestion: ConnectionRefreshSuggestion, id: number) {
  return [...current.filter(existing => existing.key !== suggestion.key), { ...suggestion, id }].slice(-3)
}

export function ConnectionRefreshPrompts({ suggestions, onDismiss, onRefreshed }: {
  suggestions: ConnectionRefreshSuggestionItem[]
  onDismiss: (id: number) => void
  onRefreshed?: () => void | Promise<void>
}) {
  if (!suggestions.length) return null
  return <aside className="connection-refresh-prompts" aria-label={t('连接刷新提示')}>
    {suggestions.map(suggestion => <ConnectionRefreshPrompt key={suggestion.id} suggestion={suggestion} onDismiss={onDismiss} onRefreshed={onRefreshed} />)}
  </aside>
}

function ConnectionRefreshPrompt({ suggestion, onDismiss, onRefreshed }: {
  suggestion: ConnectionRefreshSuggestionItem
  onDismiss: (id: number) => void
  onRefreshed?: () => void | Promise<void>
}) {
  const [state, setState] = useState<'idle' | 'busy' | 'success' | 'error'>('idle')
  const [result, setResult] = useState('')

  useEffect(() => {
    if (state !== 'success') return
    const timeout = window.setTimeout(() => onDismiss(suggestion.id), 6000)
    return () => window.clearTimeout(timeout)
  }, [state, suggestion.id, onDismiss])

  const refresh = async () => {
    setState('busy')
    setResult('')
    try {
      const response = suggestion.scope === 'gateway_local'
        ? await api.refreshLocalConnections()
        : suggestion.scope === 'policy_group'
          ? await api.refreshPolicyConnections(suggestion.group)
          : await api.refreshDeviceConnections(suggestion.deviceID)
      setResult(response.closed_connections > 0
        ? t('已关闭 {{count}} 个连接，等待客户端建立新连接。', { count: response.closed_connections })
        : t('当前没有需要刷新的连接。'))
      setState('success')
      try { await onRefreshed?.() } catch { /* The scoped refresh succeeded; overview refresh is best-effort. */ }
    } catch (cause) {
      setResult(cause instanceof Error ? cause.message : String(cause))
      setState('error')
    }
  }

  const title = state === 'success'
    ? t('{{name}} 的连接已刷新', { name: suggestion.subject })
    : state === 'error'
      ? t('出口已切换，连接刷新失败')
      : t('{{name}} 的出口已切换', { name: suggestion.subject })
  const explanation = suggestion.scope === 'gateway_local'
    ? t('新连接将使用“{{selection}}”；已有连接可能继续使用原链路。刷新会关闭 Mac 本机当前由 OpenSurge 管理的连接，下载、通话等可能中断。', { selection: suggestion.selection })
    : suggestion.scope === 'policy_group'
      ? t('经过此策略组的新连接将使用“{{selection}}”；已有连接可能继续使用原链路。刷新会关闭 Mac 本机及下游设备当前经过此组的连接，包括跟随网关规则或通过其他策略引用此组的设备。已使用新出口的连接也会关闭，下载、通话等可能中断。', { selection: suggestion.selection })
      : t('新连接将使用“{{selection}}”；已有连接可能继续使用原链路。刷新会关闭这台设备当前由 OpenSurge 管理的连接，下载、通话等可能中断。', { selection: suggestion.selection })
  const refreshLabel = suggestion.scope === 'gateway_local'
    ? t('刷新 Mac 本机连接')
    : suggestion.scope === 'policy_group'
      ? t('刷新经过此策略组的连接')
      : t('刷新 {{name}} 连接', { name: suggestion.subject })

  return <section className={`connection-refresh-prompt ${state}`} role={state === 'error' ? 'alert' : 'status'}>
    <div className="connection-refresh-prompt-heading">
      <span aria-hidden="true">{state === 'success' ? '✓' : state === 'error' ? '!' : '⇄'}</span>
      <div><strong>{title}</strong><p>{state === 'idle' || state === 'busy' ? explanation : result}</p></div>
      {state !== 'busy' && <button className="connection-refresh-prompt-close" type="button" aria-label={t('关闭提示：{{title}}', { title })} onClick={() => onDismiss(suggestion.id)}>×</button>}
    </div>
    {state !== 'success' && <div className="connection-refresh-prompt-actions">
      <button className="primary" type="button" disabled={state === 'busy'} onClick={() => void refresh()}>{state === 'busy' ? t('正在刷新…') : state === 'error' ? t('重试刷新连接') : refreshLabel}</button>
      {state !== 'busy' && <button type="button" onClick={() => onDismiss(suggestion.id)}>{t('暂不刷新')}</button>}
    </div>}
  </section>
}
