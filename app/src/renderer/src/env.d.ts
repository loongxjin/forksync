import type { EngineAPI } from './lib/api'

declare global {
  interface Window {
    api: EngineAPI
  }
}
