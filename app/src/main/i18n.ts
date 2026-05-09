/**
 * Simple i18n helper for Electron main process.
 * Reads locale from localStorage-equivalent (via electron-store or direct file).
 * Falls back to 'zh' if not set.
 *
 * Translations are loaded once at startup and cached in memory.
 * Call reloadTranslations() after changing locale.
 */

import { readFileSync, existsSync } from 'fs'
import { join } from 'path'
import { app } from 'electron'

let locale: string = 'zh'
let translations: Record<string, string> = {}
let loaded = false

// Try to detect locale from user preferences
function detectLocale(): string {
  try {
    // Check for localStorage-like config
    const configPath = join(app.getPath('userData'), 'locale.txt')
    if (existsSync(configPath)) {
      const saved = readFileSync(configPath, 'utf-8').trim()
      if (saved === 'en' || saved === 'zh') return saved
    }
  } catch {
    // ignore
  }
  return 'zh'
}

/** Flatten nested object to dot-notation keys: { "a.b": "value" } */
function flattenObject(obj: Record<string, unknown>, prefix = ''): Record<string, string> {
  const result: Record<string, string> = {}
  for (const [key, value] of Object.entries(obj)) {
    const fullKey = prefix ? `${prefix}.${key}` : key
    if (typeof value === 'string') {
      result[fullKey] = value
    } else if (typeof value === 'object' && value !== null) {
      Object.assign(result, flattenObject(value as Record<string, unknown>, fullKey))
    }
  }
  return result
}

function loadTranslations(): void {
  locale = detectLocale()
  try {
    const langPath = join(__dirname, '..', 'renderer', 'src', 'i18n', 'locales', `${locale}.json`)
    if (existsSync(langPath)) {
      const raw = readFileSync(langPath, 'utf-8')
      translations = flattenObject(JSON.parse(raw))
    }
  } catch {
    translations = {}
  }
  loaded = true
}

/**
 * Translate a key with optional interpolation.
 * Usage: t('mainProcess.selectRepoDir') or t('ide.pathNotExist', { path: '/foo' })
 *
 * Translations are cached after first load — no disk I/O on subsequent calls.
 */
export function t(key: string, params?: Record<string, string | number>): string {
  if (!loaded) loadTranslations()
  let text = translations[key] || key
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      text = text.replace(new RegExp(`\\{\\{${k}\\}\\}`, 'g'), String(v))
    }
  }
  return text
}

/** Get the current locale */
export function getLocale(): string {
  if (!loaded) loadTranslations()
  return locale
}

/** Force reload translations (call after locale change) */
export function reloadTranslations(): void {
  loaded = false
  loadTranslations()
}
