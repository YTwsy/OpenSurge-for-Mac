declare global { interface Window { __opensurgeWindowVisible?: boolean } }

// WKWebView's document visibility alone does not reliably describe an NSWindow.
// The native host adds its occlusion/minimisation state; browsers use the DOM.
export function isWindowVisible(): boolean {
  return !document.hidden && window.__opensurgeWindowVisible !== false
}

export function watchVisibleRefresh(refresh: () => void | Promise<void>, interval: number, immediate = true): () => void {
  let active = true
  let pending = false
  const run = async () => {
    if (!active || pending || !isWindowVisible()) return
    pending = true
    try { await refresh() } finally { pending = false }
  }
  const onVisible = () => { if (isWindowVisible()) void run() }
  if (immediate) void run()
  const timer = window.setInterval(() => void run(), interval)
  document.addEventListener('visibilitychange', onVisible)
  window.addEventListener('opensurge:refresh', onVisible)
  return () => {
    active = false
    window.clearInterval(timer)
    document.removeEventListener('visibilitychange', onVisible)
    window.removeEventListener('opensurge:refresh', onVisible)
  }
}
