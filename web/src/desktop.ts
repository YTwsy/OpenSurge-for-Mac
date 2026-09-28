import { isWindowVisible } from './visibility'
import { t } from './i18n'

// The marker is a per-process capability for Wails' private asset transport.
// It is never the Control Service credential or browser session cookie.
function desktopSession(): string | undefined {
  return document.querySelector<HTMLMetaElement>('meta[name="opensurge-desktop-session"]')?.content
}

export function desktopHeaders(): Record<string, string> {
  const session = desktopSession()
  return session ? { 'X-OpenSurge-Desktop': session } : {}
}

export function controlEventsURL(): string {
  const session = desktopSession()
  return session ? `/api/v1/events?desktop_session=${encodeURIComponent(session)}` : '/api/v1/events'
}

export function isDesktop(): boolean { return Boolean(desktopSession()) }

export function markUIReady() {
  document.documentElement.dataset.uiReady = 'true'
  const bridge = (window as Window & { webkit?: { messageHandlers?: { opensurgeUIReady?: { postMessage: (value: string) => void } } } }).webkit
  if (isDesktop()) bridge?.messageHandlers?.opensurgeUIReady?.postMessage('ready')
}

// WKWebView can permanently close EventSource when the private transport returns
// a reconnect error. Recreate it explicitly; never reload the page to reconnect.
export function watchControlEvents(onState: () => void): () => void {
  if (typeof EventSource === 'undefined') return () => {}
  let events: EventSource | null = null
  let timer: number | undefined
  let stopped = false
  let delay = 1000
  const connect = () => {
    if (stopped || !isWindowVisible()) return
    const source = new EventSource(controlEventsURL())
    events = source
    source.addEventListener('state', () => { if (!stopped && events === source) onState() })
    if (isDesktop()) {
      source.addEventListener('open', () => { if (events === source) delay = 1000 })
      source.addEventListener('error', () => {
        if (stopped || events !== source) return
        events = null
        source.close()
        window.clearTimeout(timer)
        if (!stopped) timer = window.setTimeout(connect, delay)
        delay = Math.min(delay * 2, 30_000)
      })
    }
  }
  const onVisible = () => {
    window.clearTimeout(timer)
    if (!isWindowVisible()) { events?.close(); events = null }
    else if (!events) { delay = 1000; connect() }
  }
  document.addEventListener('visibilitychange', onVisible)
  connect()
  return () => { stopped = true; window.clearTimeout(timer); events?.close(); document.removeEventListener('visibilitychange', onVisible) }
}

export async function desktopAction<T = { ok: boolean }>(action: string, body: Record<string, unknown> = {}): Promise<T> {
  const response = await fetch(`/desktop/v1/${action}`, { method: 'POST', headers: { 'Content-Type': 'application/json', ...desktopHeaders() }, body: JSON.stringify(body) })
  if (!response.ok) {
    const failure = await response.json().catch(() => null) as { error?: { code?: string } } | null
    const messages: Record<string, string> = {
      desktop_browser_failed: '无法在浏览器中打开。请确认后台服务可用后重试。',
      desktop_exit_unsafe: '请先在网络设置中停止网关并完成恢复。',
      desktop_uninstall_unsafe: '请先在网络设置中停止网关，再卸载 OpenSurge。',
      desktop_uninstall_unavailable: '当前 App 无法卸载已安装的 OpenSurge。',
      desktop_uninstall_login: '无法关闭登录项。请在系统设置中关闭后重试卸载。',
      desktop_uninstall_restore: '卸载未完成，登录项也未能恢复。请检查系统登录项设置。',
      desktop_uninstall_failed: '卸载未完成。请重新连接后台服务并检查安装状态。',
    }
    throw new Error(t(messages[failure?.error?.code ?? ''] ?? '桌面操作未完成，请重试。'))
  }
  return response.json() as Promise<T>
}

export async function copyText(value: string): Promise<void> {
  if (isDesktop()) { await desktopAction('copy-text', { text: value }); return }
  if (navigator.clipboard?.writeText) { await navigator.clipboard.writeText(value); return }
  const field = document.createElement('textarea')
  field.value = value
  field.setAttribute('readonly', '')
  field.style.position = 'fixed'
  field.style.opacity = '0'
  document.body.appendChild(field)
  field.select()
  const copied = document.execCommand('copy')
  field.remove()
  if (!copied) throw new Error(t('浏览器未允许复制路径，请使用 Finder 中显示。'))
}

export function watchDesktopLinks(onError: (message: string) => void): () => void {
  if (!isDesktop()) return () => {}
  const click = (event: MouseEvent) => {
    const target = event.target instanceof Element ? event.target.closest('a[href]') : null
    if (!(target instanceof HTMLAnchorElement)) return
    const url = new URL(target.href)
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return
    event.preventDefault()
    void desktopAction('open-external', { url: url.href }).catch(cause => onError(String(cause.message)))
  }
  document.addEventListener('click', click)
  return () => document.removeEventListener('click', click)
}
