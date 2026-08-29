import { spawn } from 'node:child_process'

// Keep the implementation-specific process command outside the HTTP contract.
// A future Gateway changes this adapter without rewriting the assertions.
export function launchWebEntry(cwd) {
  return spawn(process.execPath, ['dist/main.js'], {
    cwd,
    env: { NODE_ENV: 'production', TZ: 'UTC' },
    stdio: ['ignore', 'pipe', 'pipe']
  })
}
