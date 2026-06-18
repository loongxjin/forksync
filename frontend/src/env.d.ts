import type { EngineAPI } from './lib/api'

declare global {
  interface Window {
    api: EngineAPI
    platform: string
    ipcSend: (channel: 'window:minimize' | 'window:maximize' | 'window:close') => void
  }
}
