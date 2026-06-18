import { app, shell, BrowserWindow, nativeImage, session } from 'electron'
import { join } from 'path'
import { electronApp, optimizer, is } from '@electron-toolkit/utils'
import { registerIpcHandlers } from './ipc'
import { registerIDEHandlers } from './ide'
import { injectShellPath } from './shell-path'
import { getEngineServer } from './server'
import log from './logger'

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
      sandbox: true,
      contextIsolation: true,
      nodeIntegration: false
    },
    ...platformWindowOptions
  })

  mainWindow.on('ready-to-show', () => {
    mainWindow.show()
  })

  mainWindow.webContents.setWindowOpenHandler((details) => {
    const url = new URL(details.url)
    if (url.protocol === 'http:' || url.protocol === 'https:') {
      shell.openExternal(details.url)
    }
    return { action: 'deny' }
  })

  // Set Content-Security-Policy.
  //
  // connect-src: in production the renderer NEVER talks to the engine directly
  // — every request flows through the main process via IPC (api.ts → preload →
  // ipc-engine.ts → engine.ts). So the renderer's connect-src only needs
  // 'self' in production, denying a compromised renderer any outbound network
  // exfiltration channel. In dev, Vite HMR needs ws connections to its dev
  // server on 127.0.0.1/localhost, so we relax connect-src only there.
  //
  // script-src keeps 'unsafe-inline' for the synchronous theme-init script in
  // index.html (FOUC prevention). Tightening that requires a per-request nonce
  // + HTML rewrite, which is a follow-up.
  const devConnect = is.dev
    ? ' http://127.0.0.1:* ws://127.0.0.1:* http://localhost:* ws://localhost:*'
    : ''
  mainWindow.webContents.session.webRequest.onHeadersReceived((details, callback) => {
    callback({
      responseHeaders: {
        ...details.responseHeaders,
        'Content-Security-Policy': [
          "default-src 'self'" +
            "; script-src 'self' 'unsafe-inline'" + (is.dev ? " 'unsafe-eval'" : "") +
            "; style-src 'self' 'unsafe-inline'" +
            `; connect-src 'self'${devConnect}`
        ]
      }
    })
  })

  if (is.dev && process.env['ELECTRON_RENDERER_URL']) {
    mainWindow.loadURL(process.env['ELECTRON_RENDERER_URL'])
  } else {
    mainWindow.loadFile(join(__dirname, '../renderer/index.html'))
  }
}

app.whenReady().then(async () => {
  // Inject user's shell PATH so the packaged app can find CLI tools
  // (e.g. claude, opencode) that live outside /usr/bin:/bin:/usr/sbin:/sbin.
  injectShellPath()

  electronApp.setAppUserModelId('com.forksync.app')

  // Set macOS dock icon for dev mode
  if (process.platform === 'darwin') {
    app.dock.setIcon(nativeImage.createFromPath(join(__dirname, '../../resources/icon.png')))
  }

  // Start the embedded Go HTTP server before registering handlers so the
  // first engine call (e.g. updateNotificationConfig) doesn't stall on the
  // process spawn. Failures here are non-fatal: EngineClient will retry the
  // spawn lazily on demand.
  try {
    await getEngineServer().getBaseUrl()
  } catch (err) {
    log.error('[main] engine server failed to start at boot:', (err as Error).message)
  }

  // Broadcast engine lifecycle status (starting/ready/reconnecting/down) to the
  // renderer so it can show a "reconnecting" banner when the engine crashes.
  // The supervisor auto-restarts with backoff; this only drives the UI.
  getEngineServer().onStatusChange((status) => {
    for (const win of BrowserWindow.getAllWindows()) {
      win.webContents.send('engine:status', status)
    }
  })

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

// Stop the embedded Go HTTP server on quit (the existing before-quit hook in
// ipc-engine.ts only kills resolve streams; this terminates the server itself).
app.on('will-quit', () => {
  getEngineServer().kill()
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit()
  }
})
