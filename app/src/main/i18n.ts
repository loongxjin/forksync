/**
 * Simple i18n helper for Electron main process.
 * Reads locale from localStorage-equivalent (via electron-store or direct file).
 * Falls back to 'en' if not set.
 */

import { readFileSync, existsSync } from 'fs'
import { join } from 'path'
import { app } from 'electron'

let locale: string = 'zh'

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

let translations: Record<string, string> = {}

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

/**
 * Translate a key with optional interpolation.
 * Usage: t('mainProcess.selectRepoDir') or t('ide.pathNotExist', { path: '/foo' })
 */
export function t(key: string, params?: Record<string, string | number>): string {
  loadTranslations()
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
  loadTranslations()
  return locale
}
