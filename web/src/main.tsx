import { StrictMode, useEffect, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { activateLanguage, initialRequestedLanguage, prepareLanguage } from './i18n'
import { isDesktop, markUIReady } from './desktop'

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

  const App = isDesktop() && window.location.pathname === '/desktop-tray'
    ? (await import('./tray/TrayApp')).TrayApp
    : isDesktop() && window.location.pathname === '/desktop-tray-menu'
    ? (await import('./tray/TrayApp')).TrayMenuApp
    : isDesktop() && window.location.pathname === '/desktop-settings'
    ? (await import('./settings/SettingsApp')).SettingsApp
    : isDesktop() ? (await import('./App')).App : (await import('./BrowserApp')).BrowserApp

  createRoot(document.getElementById('root')!).render(
    <StrictMode><FirstFrame><App /></FirstFrame></StrictMode>,
  )
}

void start().catch(() => {
  const error = document.getElementById('startup-error')
  if (error) error.hidden = false
  markUIReady()
})
