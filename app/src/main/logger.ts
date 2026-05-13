/**
 * Application logger — thin wrapper around electron-log.
 *
 * Production (packaged) → Info level, file output only.
 * Development           → Debug level, console + file output.
 *
 * Usage:
 *   import log from './logger'
 *   log.info('message', data)
 *   log.warn('message', data)
 *   log.error('message', data)
 *   log.debug('message', data)   // only visible in dev
 */

import { app } from 'electron'
import log from 'electron-log'
import { join } from 'path'
import { homedir } from 'os'

const isDev = !app.isPackaged

// Configure electron-log
log.transports.file.resolvePathFn = () => {
  return join(homedir(), '.forksync', 'logs', 'electron-main.log')
}

// In production: only write to file at Info level
// In development: write to console + file at Debug level
log.transports.console.level = isDev ? 'debug' : false
log.transports.file.level = isDev ? 'debug' : 'info'

// Format for file output
log.transports.file.format = '[{y}-{m}-{d} {h}:{i}:{s}.{ms}] [{level}] {text}'

// Log file size limit: 5MB, keep 3 rotated files
log.transports.file.maxSize = 5 * 1024 * 1024

export default log
