import { useEffect, useState, type SetStateAction } from 'react'

export type Theme = 'dark' | 'light'
export type ThemePreference = Theme | 'system'
const key = 'opensurge-theme'
const valid = (value: unknown): value is ThemePreference => value === 'dark' || value === 'light' || value === 'system'
const systemTheme = (): Theme => typeof window.matchMedia === 'function' && window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'

function initialPreference(): ThemePreference {
 const stored = window.localStorage.getItem(key)
 return valid(stored) ? stored : 'system'
}

// Native windows share an origin. Preserve explicit choices, and keep following
// macOS when the user chooses System instead of freezing its current appearance.
export function useTheme() {
 const [preference, setPreference] = useState<ThemePreference>(initialPreference)
 const [system, setSystem] = useState(systemTheme)
 const theme = preference === 'system' ? system : preference
 useEffect(() => {
  document.documentElement.dataset.theme = theme
  window.localStorage.setItem(key, preference)
 }, [theme, preference])
 useEffect(() => {
  const media = window.matchMedia?.('(prefers-color-scheme: light)')
  const appearanceChanged = () => setSystem(systemTheme())
  const changed = (event: StorageEvent) => {
   if (event.key === key && valid(event.newValue)) setPreference(event.newValue)
  }
  media?.addEventListener('change', appearanceChanged)
  window.addEventListener('storage', changed)
  return () => { media?.removeEventListener('change', appearanceChanged); window.removeEventListener('storage', changed) }
 }, [])
 const setTheme = (value: SetStateAction<Theme>) => setPreference(typeof value === 'function' ? value(theme) : value)
 return [theme, setTheme, preference, setPreference] as const
}
