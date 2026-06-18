import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import zh from './locales/zh.json'
import en from './locales/en.json'

let savedLocale = 'zh'
try {
  savedLocale = localStorage.getItem('forksync-locale') || 'zh'
} catch {
  // localStorage unavailable (e.g. restricted environment)
}

i18n.use(initReactI18next).init({
  resources: {
    zh: { translation: zh },
    en: { translation: en }
  },
  lng: savedLocale,
  fallbackLng: 'zh',
  interpolation: {
    escapeValue: false // React already escapes
  }
})

export default i18n
