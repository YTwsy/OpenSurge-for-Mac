import { StrictMode, useEffect, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { activateLanguage, initialRequestedLanguage, prepareLanguage } from './i18n'
import { markUIReady } from './desktop'

function FirstFrame({ children }: { children: ReactNode }) {
  useEffect(() => {
    let frame = requestAnimationFrame(() => { frame = requestAnimationFrame(markUIReady) })
    return () => cancelAnimationFrame(frame)
  }, [])
  return children
}

async function start() {
  const language = initialRequestedLanguage()
  await prepareLanguage(language)
  activateLanguage(language)

  const App = window.location.pathname === '/desktop-tray'
    ? (await import('./tray/TrayApp')).TrayApp
    : window.location.pathname === '/desktop-settings'
    ? (await import('./settings/SettingsApp')).SettingsApp
    : (await import('./App')).App

  createRoot(document.getElementById('root')!).render(
    <StrictMode><FirstFrame><App /></FirstFrame></StrictMode>,
  )
}

void start().catch(() => {
  const error = document.getElementById('startup-error')
  if (error) error.hidden = false
  markUIReady()
})
