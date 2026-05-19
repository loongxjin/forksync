/**
 * IPC Handlers — registration orchestrator
 *
 * Splits IPC registration into focused modules:
 * - ipc-engine.ts: engine commands and streaming
 * - ipc-app.ts: auto-launch, dialogs, filesystem
 * - ipc-window.ts: window controls
 */

import { registerEngineIpcHandlers } from './ipc-engine'
import { registerAppIpcHandlers } from './ipc-app'
import { registerWindowIpcHandlers } from './ipc-window'

export function registerIpcHandlers(): void {
  registerEngineIpcHandlers()
  registerAppIpcHandlers()
  registerWindowIpcHandlers()
}

// Re-export validation helpers for use by ide.ts
export { assertString, assertOptionalString, assertSafePath } from './ipc-engine'
