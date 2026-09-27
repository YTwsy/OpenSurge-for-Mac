import { useEffect, useState } from 'react'

export type Theme = 'dark' | 'light'
const key = 'opensurge-theme'

function initialTheme(): Theme {
 const stored = window.localStorage.getItem(key)
 if (stored === 'dark' || stored === 'light') return stored
 return typeof window.matchMedia === 'function' && window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
}

// Both native WebViews share the same origin and preference. The browser keeps
// its existing preference, and changes in another window are applied live.
export function useTheme() {
 const [theme, setTheme] = useState<Theme>(initialTheme)
 useEffect(() => {
  document.documentElement.dataset.theme = theme
  window.localStorage.setItem(key, theme)
 }, [theme])
 useEffect(() => {
  const changed = (event: StorageEvent) => {
   if (event.key === key && (event.newValue === 'dark' || event.newValue === 'light')) setTheme(event.newValue)
  }
  window.addEventListener('storage', changed)
  return () => window.removeEventListener('storage', changed)
 }, [])
 return [theme, setTheme] as const
}
