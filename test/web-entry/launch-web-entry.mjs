import { spawn } from 'node:child_process'
import path from 'node:path'

// Keep the implementation-specific process command outside the HTTP contract.
// A future Gateway changes this adapter without rewriting the assertions.
export function launchWebEntry({
  cwd,
  listenAddress,
  backendURL,
  staticDir = path.join(cwd, 'public'),
  proxyTimeout = '150ms',
  basePath,
  frameAncestors,
  binary = path.join(cwd, 'server', 'build', 'web-entry')
}) {
  const args = [
    `--listen-address=${listenAddress}`,
    `--backend-url=${backendURL}`,
    `--static-dir=${staticDir}`,
    `--proxy-timeout=${proxyTimeout}`
  ]
  if (basePath !== undefined) args.push(`--base-path=${basePath}`)
  if (frameAncestors !== undefined) {
    args.push(`--frame-ancestors-json=${JSON.stringify(frameAncestors)}`)
  }

  return spawn(binary, args, {
    cwd,
    env: { TZ: 'UTC' },
    stdio: ['ignore', 'pipe', 'pipe']
  })
}
