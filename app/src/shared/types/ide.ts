/**
 * IDE types — IDE detection, configuration, and management
 */

export interface IDEInfo {
  id: string // 'vscode' | 'cursor' | 'trae' | 'custom-xxx'
  name: string // display name: 'VS Code' | 'Cursor' | 'Trae'
  cliCommand: string // CLI command: 'code' | 'cursor' | 'trae'
  appName: string // macOS app name: 'Visual Studio Code' | 'Cursor' | 'Trae'
  installed: boolean // whether IDE is detected as installed
  openMethod: 'cli' | 'app' | 'flatpak' // open via CLI command, `open -a`, or flatpak
  isCustom?: boolean // user-added custom IDE
}

export interface CustomIDE {
  id: string // 'custom-<timestamp>'
  name: string
  cliCommand: string
}

export interface IDEConfig {
  defaultIDE: string | null // default IDE id, null = not set
  detectedIDEs: IDEInfo[] // detection results
  customIDEs: CustomIDE[] // user-added custom IDEs
}

export interface IDEOpenResult {
  success: boolean
  error?: string
}
