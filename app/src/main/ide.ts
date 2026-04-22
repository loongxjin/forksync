/**
 * IDE detection and management — main process module
 *
 * Detects installed IDEs (VSCode, Cursor, Trae), manages IDE config,
 * and opens projects in the selected IDE.
 */

import { ipcMain } from 'electron'
import { t } from './i18n'
import { execFile } from 'child_process'
import { existsSync, readFileSync, writeFileSync, mkdirSync } from 'fs'
import { join } from 'path'
import { homedir } from 'os'
import type { IDEInfo, CustomIDE, IDEConfig, IDEOpenResult } from '../renderer/src/types/ide'

// ---------------------------------------------------------------------------
// Built-in IDE definitions
// ---------------------------------------------------------------------------

const BUILTIN_IDES: Omit<IDEInfo, 'installed' | 'openMethod'>[] = [
  {
    id: 'vscode',
    name: 'VS Code',
    cliCommand: 'code',
    appName: 'Visual Studio Code'
  },
  {
    id: 'cursor',
    name: 'Cursor',
    cliCommand: 'cursor',
    appName: 'Cursor'
  },
  {
    id: 'trae',
    name: 'Trae',
    cliCommand: 'trae',
    appName: 'Trae'
  }
]

// ---------------------------------------------------------------------------
// Config persistence
// ---------------------------------------------------------------------------

const CONFIG_DIR = join(homedir(), '.forksync')
const CONFIG_PATH = join(CONFIG_DIR, 'ide-config.json')

interface PersistedConfig {
  defaultIDE: string | null
  customIDEs: CustomIDE[]
}

function loadPersistedConfig(): PersistedConfig {
  try {
    if (existsSync(CONFIG_PATH)) {
      const raw = readFileSync(CONFIG_PATH, 'utf-8')
      const parsed = JSON.parse(raw)
      return {
        defaultIDE: parsed.defaultIDE ?? null,
        customIDEs: parsed.customIDEs ?? []
      }
    }
  } catch {
    // corrupted file — return defaults
  }
  return { defaultIDE: null, customIDEs: [] }
}

function savePersistedConfig(config: PersistedConfig): void {
  try {
    if (!existsSync(CONFIG_DIR)) {
      mkdirSync(CONFIG_DIR, { recursive: true })
    }
    writeFileSync(CONFIG_PATH, JSON.stringify(config, null, 2), 'utf-8')
  } catch {
    // silent fail — non-critical
  }
}

// ---------------------------------------------------------------------------
// Cross-platform helpers
// ---------------------------------------------------------------------------

function getWhichCommand(): string {
  return process.platform === 'win32' ? 'where' : 'which'
}

function execAsync(cmd: string, args: string[]): Promise<string> {
  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => reject(new Error('timeout')), 3000)
    execFile(cmd, args, (err, stdout) => {
      clearTimeout(timeout)
      if (err) reject(err)
      else resolve(stdout.trim())
    })
  })
}

// ---------------------------------------------------------------------------
// IDE detection
// ---------------------------------------------------------------------------

async function detectSingleIDE(ide: Omit<IDEInfo, 'installed' | 'openMethod'>): Promise<IDEInfo> {
  // 1. Try CLI via which/where
  try {
    await execAsync(getWhichCommand(), [ide.cliCommand])
    return { ...ide, installed: true, openMethod: 'cli' }
  } catch {
    // CLI not in PATH, try app detection
  }

  // 2. Platform-specific app detection
  if (process.platform === 'darwin') {
    const appPath = `/Applications/${ide.appName}.app`
    if (existsSync(appPath)) {
      return { ...ide, installed: true, openMethod: 'app' }
    }
  } else if (process.platform === 'win32') {
    const localAppData = process.env.LOCALAPPDATA
    const programFiles = process.env.PROGRAMFILES
    const programFilesX86 = process.env['PROGRAMFILES(X86)']

    const windowsPaths: Record<string, string[]> = {
      vscode: [
        localAppData ? join(localAppData, 'Programs', 'Microsoft VS Code', 'bin', 'code.cmd') : '',
        programFiles ? join(programFiles, 'Microsoft VS Code', 'bin', 'code.cmd') : '',
        programFilesX86 ? join(programFilesX86, 'Microsoft VS Code', 'bin', 'code.cmd') : ''
      ],
      cursor: [
        localAppData ? join(localAppData, 'Programs', 'cursor', 'Cursor.exe') : '',
        localAppData ? join(localAppData, 'Programs', 'Cursor', 'Cursor.exe') : '',
        programFiles ? join(programFiles, 'Cursor', 'Cursor.exe') : '',
        programFilesX86 ? join(programFilesX86, 'Cursor', 'Cursor.exe') : ''
      ],
      trae: [
        localAppData ? join(localAppData, 'Programs', 'Trae', 'Trae.exe') : '',
        programFiles ? join(programFiles, 'Trae', 'Trae.exe') : '',
        programFilesX86 ? join(programFilesX86, 'Trae', 'Trae.exe') : ''
      ]
    }

    const paths = windowsPaths[ide.id] || []
    for (const p of paths) {
      if (p && existsSync(p)) {
        return { ...ide, installed: true, openMethod: 'cli', cliCommand: p }
      }
    }
  } else if (process.platform === 'linux') {
    // Check snap/flatpak as fallbacks
    const snapPath = `/snap/bin/${ide.cliCommand}`
    if (existsSync(snapPath)) {
      return { ...ide, installed: true, openMethod: 'cli', cliCommand: snapPath }
    }

    const flatpakIds: Record<string, string> = {
      vscode: 'com.visualstudio.code'
      // cursor and trae flatpak ids are not commonly available yet
    }
    const flatpakId = flatpakIds[ide.id]
    if (flatpakId && existsSync('/usr/bin/flatpak')) {
      try {
        await execAsync('flatpak', ['info', flatpakId])
        // Store flatpak command and args separately for proper execFile usage
        return { ...ide, installed: true, openMethod: 'flatpak', cliCommand: `flatpak:${flatpakId}` }
      } catch {
        // not installed via flatpak
      }
    }
  }

  return { ...ide, installed: false, openMethod: 'cli' }
}

async function detectCustomIDE(custom: CustomIDE): Promise<IDEInfo> {
  try {
    await execAsync(getWhichCommand(), [custom.cliCommand])
    return {
      id: custom.id,
      name: custom.name,
      cliCommand: custom.cliCommand,
      appName: custom.name,
      installed: true,
      openMethod: 'cli',
      isCustom: true
    }
  } catch {
    return {
      id: custom.id,
      name: custom.name,
      cliCommand: custom.cliCommand,
      appName: custom.name,
      installed: false,
      openMethod: 'cli',
      isCustom: true
    }
  }
}

/** Cached detection results */
let cachedDetectedIDEs: IDEInfo[] | null = null

async function detectAllIDEs(): Promise<IDEInfo[]> {
  const config = loadPersistedConfig()

  const builtin = await Promise.all(BUILTIN_IDES.map((ide) => detectSingleIDE(ide)))
  const customs = await Promise.all(config.customIDEs.map((c) => detectCustomIDE(c)))

  cachedDetectedIDEs = [...builtin, ...customs]
  return cachedDetectedIDEs
}

// ---------------------------------------------------------------------------
// Open project in IDE
// ---------------------------------------------------------------------------

async function openInIDE(repoPath: string, ideId: string): Promise<IDEOpenResult> {
  if (!existsSync(repoPath)) {
    return { success: false, error: t('ide.pathNotExist', { path: repoPath }) }
  }

  // Find IDE info from cache or re-detect
  const ides = cachedDetectedIDEs ?? (await detectAllIDEs())
  const ide = ides.find((i) => i.id === ideId)
  if (!ide) {
    return { success: false, error: t('ide.ideNotFound', { ideId }) }
  }
  if (!ide.installed) {
    return { success: false, error: t('ide.ideNotDetected', { name: ide.name }) }
  }

  try {
    if (ide.openMethod === 'cli') {
      const child = execFile(ide.cliCommand, [repoPath], (err) => {
        if (err) {
          console.error(`Failed to open ${repoPath} with ${ide.name}:`, err)
        }
      })
      child.unref()
    } else if (ide.openMethod === 'flatpak') {
      // cliCommand is stored as 'flatpak:<id>', extract the flatpak app ID
      const flatpakId = ide.cliCommand.replace('flatpak:', '')
      const child = execFile('flatpak', ['run', flatpakId, repoPath], (err) => {
        if (err) {
          console.error(`Failed to open ${repoPath} with ${ide.name} via flatpak:`, err)
        }
      })
      child.unref()
    } else if (process.platform === 'darwin') {
      const child = execFile('open', ['-a', ide.appName, repoPath], (err) => {
        if (err) {
          console.error(`Failed to open ${repoPath} with ${ide.name}:`, err)
        }
      })
      child.unref()
    } else if (process.platform === 'win32') {
      const child = execFile('cmd', ['/c', 'start', '', ide.appName || ide.name, repoPath], (err) => {
        if (err) {
          console.error(`Failed to open ${repoPath} with ${ide.name}:`, err)
        }
      })
      child.unref()
    } else {
      // Linux: app-based openMethod is not supported
      return { success: false, error: t('ide.openMethodNotSupported', { method: ide.openMethod }) }
    }
    return { success: true }
  } catch (err) {
    return {
      success: false,
      error: err instanceof Error ? err.message : String(err)
    }
  }
}

// ---------------------------------------------------------------------------
// IPC Handlers
// ---------------------------------------------------------------------------

export function registerIDEHandlers(): void {
  ipcMain.handle('ide:detect', async () => {
    const ides = await detectAllIDEs()
    return ides
  })

  ipcMain.handle('ide:open', async (_event, repoPath: string, ideId: string) => {
    return openInIDE(repoPath, ideId)
  })

  ipcMain.handle('ide:getConfig', async () => {
    const ides = cachedDetectedIDEs ?? (await detectAllIDEs())
    const config = loadPersistedConfig()

    // Validate defaultIDE still exists
    let defaultIDE = config.defaultIDE
    if (defaultIDE) {
      const exists = ides.find((i) => i.id === defaultIDE && i.installed)
      if (!exists) {
        defaultIDE = null
        savePersistedConfig({ ...config, defaultIDE: null })
      }
    }

    return {
      defaultIDE,
      detectedIDEs: ides,
      customIDEs: config.customIDEs
    } satisfies IDEConfig
  })

  ipcMain.handle('ide:setDefault', async (_event, ideId: string | null) => {
    const config = loadPersistedConfig()
    config.defaultIDE = ideId
    savePersistedConfig(config)
    return { success: true }
  })

  ipcMain.handle('ide:addCustom', async (_event, name: string, cliCommand: string) => {
    const config = loadPersistedConfig()
    const newCustom: CustomIDE = {
      id: `custom-${Date.now()}`,
      name,
      cliCommand
    }
    config.customIDEs.push(newCustom)
    savePersistedConfig(config)

    // Re-detect to include new custom IDE
    cachedDetectedIDEs = null
    await detectAllIDEs()

    return { success: true }
  })

  ipcMain.handle('ide:removeCustom', async (_event, ideId: string) => {
    const config = loadPersistedConfig()
    config.customIDEs = config.customIDEs.filter((c) => c.id !== ideId)
    if (config.defaultIDE === ideId) {
      config.defaultIDE = null
    }
    savePersistedConfig(config)

    // Re-detect
    cachedDetectedIDEs = null
    await detectAllIDEs()

    return { success: true }
  })
}
