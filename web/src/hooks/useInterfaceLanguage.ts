import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '../api'
import { desktopAction, isDesktop } from '../desktop'
import { activateLanguage, cacheRequestedLanguage, initialRequestedLanguage, isRequestedLanguage, languageCacheKey, prepareLanguage, resolveLanguage, type RequestedLanguage } from '../i18n'

// Preferences have one Control API owner. Fence background reads across saves
// and catalog loads so another window or an old response cannot undo a change.
export function useInterfaceLanguage(onError: (message: string) => void, optimistic = true) {
 const [language, setLanguage] = useState(initialRequestedLanguage)
 const [languageChanging, setChanging] = useState(false)
 const generation = useRef(0)
 const pending = useRef(false)
 const active = useRef(true)
 useEffect(() => { active.current = true; return () => { active.current = false } }, [])
 useEffect(() => {
  activateLanguage(language)
  cacheRequestedLanguage(language)
  if (isDesktop()) void desktopAction('language', { language: resolveLanguage(language) }).catch(() => {})
 }, [language])
 const commit = useCallback(async (value: RequestedLanguage, revision: number) => {
  await prepareLanguage(value)
  if (!active.current || revision !== generation.current) return
  activateLanguage(value)
  setLanguage(value)
 }, [])
 const beginLanguageRefresh = useCallback(() => {
  const revision = generation.current
  return async (value: unknown) => {
   if (!pending.current && revision === generation.current && isRequestedLanguage(value)) await commit(value, revision)
  }
 }, [commit])
 useEffect(() => {
  const changed = (event: StorageEvent) => {
   if (event.key !== languageCacheKey || !isRequestedLanguage(event.newValue) || pending.current) return
   void commit(event.newValue, ++generation.current).catch(() => {})
  }
  window.addEventListener('storage', changed)
  return () => window.removeEventListener('storage', changed)
 }, [commit])
 const changeLanguage = async (value: RequestedLanguage) => {
  if (pending.current || value === language) return false
  const previous = language
  const revision = ++generation.current
  pending.current = true; setChanging(true)
  try {
   if (optimistic) await commit(value, revision)
   const preferences = await api.setUIPreferences({ language: value })
   await commit(preferences.language, ++generation.current)
   return true
  } catch (cause) {
   await commit(previous, ++generation.current)
   if (active.current) onError(cause instanceof Error ? cause.message : String(cause))
   return false
  } finally { pending.current = false; if (active.current) setChanging(false) }
 }
 return { language, languageChanging, changeLanguage, beginLanguageRefresh }
}
