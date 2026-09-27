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

// WKWebView can permanently close EventSource when the private transport returns
// a reconnect error. Recreate it explicitly; never reload the page to reconnect.
export function watchControlEvents(onState: () => void): () => void {
  if (typeof EventSource === 'undefined') return () => {}
  let events: EventSource | null = null
  let timer: number | undefined
  let stopped = false
  let delay = 1000
  const connect = () => {
    if (stopped) return
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
  connect()
  return () => { stopped = true; window.clearTimeout(timer); events?.close() }
}
