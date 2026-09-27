import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { activateLanguage, initialRequestedLanguage, prepareLanguage } from './i18n'

async function start() {
  const language = initialRequestedLanguage()
  await prepareLanguage(language)
  activateLanguage(language)

  const App = window.location.pathname === '/desktop-tray'
    ? (await import('./tray/TrayApp')).TrayApp
    : (await import('./App')).App

  createRoot(document.getElementById('root')!).render(
    <StrictMode><App /></StrictMode>,
  )
}

void start()
