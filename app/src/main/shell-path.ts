/**
 * Shell PATH helper — injects the user's interactive shell PATH into process.env.
 *
 * On macOS, GUI apps launched from Finder/Dock inherit a minimal system PATH
 * (/usr/bin:/bin:/usr/sbin:/sbin) and do NOT pick up the user's shell PATH
 * (e.g. /opt/homebrew/bin, /usr/local/bin, ~/.nvm/...).
 * This causes the Go engine's exec.LookPath() to fail for agents installed
 * outside the system directories.
 */

import { execFileSync } from 'child_process'
import { app } from 'electron'

export function injectShellPath(): void {
  // Only needed for packaged macOS / Linux builds. Dev mode launched from a
  // terminal already inherits the full shell PATH. Windows uses registry-based
  // environment variables and generally does not have this issue.
  if (
    !app.isPackaged ||
    (process.platform !== 'darwin' && process.platform !== 'linux')
  ) {
    return
  }

  try {
    const shell = process.env.SHELL || '/bin/sh'
    const pathOutput = execFileSync(shell, ['-ilc', 'printf "%s" "$PATH"'], {
      encoding: 'utf-8',
      timeout: 5000
    })
    const shellPath = pathOutput.trim()

    if (shellPath && shellPath !== process.env.PATH) {
      process.env.PATH = shellPath
    }
  } catch (err) {
    console.error('Failed to inject shell PATH:', err)
  }
}
