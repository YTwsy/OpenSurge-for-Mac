import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import { activateLanguage, initialRequestedLanguage, prepareLanguage } from './i18n'
import './styles.css'

async function start() {
  const language = initialRequestedLanguage()
  await prepareLanguage(language)
  activateLanguage(language)

  createRoot(document.getElementById('root')!).render(
    <StrictMode><App /></StrictMode>,
  )
}

void start()
