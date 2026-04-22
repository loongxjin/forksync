import { app, shell, BrowserWindow, nativeImage } from 'electron'
import { join } from 'path'
import { electronApp, optimizer, is } from '@electron-toolkit/utils'
import { registerIpcHandlers } from './ipc'
import { registerIDEHandlers } from './ide'
import { injectShellPath } from './shell-path'

function createWindow(): void {
  const platform = process.platform

  // Build platform-specific window options
  const platformWindowOptions: Electron.BrowserWindowConstructorOptions = {}

  if (platform === 'darwin') {
    // macOS: use hiddenInset title bar (traffic lights visible)
    platformWindowOptions.titleBarStyle = 'hiddenInset'
  } else if (platform === 'win32') {
    // Windows: frameless with native window control overlay
    platformWindowOptions.frame = false
    platformWindowOptions.titleBarOverlay = {
      color: '#0c1222',
      symbolColor: '#c8d6e5',
      height: 38
    }
  } else {
    // Linux: frameless, window controls rendered in TitleBar component
    platformWindowOptions.frame = false
  }

  const mainWindow = new BrowserWindow({
    width: 1200,
    height: 800,
    minWidth: 900,
    minHeight: 600,
    show: false,
    title: 'ForkSync',
    icon: nativeImage.createFromPath(join(__dirname, '../../resources/icon.png')),
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      sandbox: false
    },
    ...platformWindowOptions
  })

  mainWindow.on('ready-to-show', () => {
    mainWindow.show()
  })

  mainWindow.webContents.setWindowOpenHandler((details) => {
    shell.openExternal(details.url)
    return { action: 'deny' }
  })

  if (is.dev && process.env['ELECTRON_RENDERER_URL']) {
    mainWindow.loadURL(process.env['ELECTRON_RENDERER_URL'])
  } else {
    mainWindow.loadFile(join(__dirname, '../renderer/index.html'))
  }
}

app.whenReady().then(() => {
  // Inject user's shell PATH so the packaged app can find CLI tools
  // (e.g. claude, opencode) that live outside /usr/bin:/bin:/usr/sbin:/sbin.
  injectShellPath()

  electronApp.setAppUserModelId('com.forksync.app')

  // Set macOS dock icon for dev mode
  if (process.platform === 'darwin') {
    app.dock.setIcon(nativeImage.createFromPath(join(__dirname, '../../resources/icon.png')))
  }

  // Register IPC handlers for engine communication
  registerIpcHandlers()
  registerIDEHandlers()

  app.on('browser-window-created', (_, window) => {
    optimizer.watchWindowShortcuts(window)
  })

  createWindow()

  app.on('activate', function () {
    if (BrowserWindow.getAllWindows().length === 0) createWindow()
  })
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit()
  }
})
