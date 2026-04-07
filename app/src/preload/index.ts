import { contextBridge } from 'electron'

if (process.contextIsolated) {
  contextBridge.exposeInMainWorld('api', {})
} else {
  ;(window as any).api = {}
}
