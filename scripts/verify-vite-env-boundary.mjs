import { spawnSync } from 'node:child_process'
import { readdir, readFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const repositoryRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  '..'
)
const outputDirectory = path.join(repositoryRoot, 'public')

const inheritedEnvironment = ['CI', 'HOME', 'PATH', 'PNPM_HOME', 'TMPDIR']
const buildEnvironment = Object.fromEntries(
  inheritedEnvironment
    .filter((name) => process.env[name] !== undefined)
    .map((name) => [name, process.env[name]])
)
Object.assign(buildEnvironment, {
  HAMI_WEBUI_UNREFERENCED_CANARY: 'hami_webui_canary_should_not_ship',
  VITE_APP_BASE_API: '/vite-env-contract/',
  VITE_APP_REQUEST_TIMEOUT: '71234567'
})

const pnpm = process.platform === 'win32' ? 'pnpm.cmd' : 'pnpm'
const build = spawnSync(
  pnpm,
  ['--filter', 'hami-webui-web', 'run', 'build'],
  {
    cwd: repositoryRoot,
    env: buildEnvironment,
    stdio: 'inherit'
  }
)
if (build.status !== 0) {
  throw new Error(`Vite build failed with status ${build.status}`)
}

const listJavaScriptFiles = async(directory) => {
  const entries = await readdir(directory, { withFileTypes: true })
  const files = await Promise.all(
    entries.map((entry) => {
      const entryPath = path.join(directory, entry.name)
      return entry.isDirectory() ? listJavaScriptFiles(entryPath) : [entryPath]
    })
  )

  return files.flat().filter((file) => file.endsWith('.js'))
}

const bundles = await Promise.all(
  (await listJavaScriptFiles(outputDirectory)).map((file) =>
    readFile(file, 'utf8')
  )
)
const contains = (value) => bundles.some((bundle) => bundle.includes(value))

const forbiddenValues = [
  'HAMI_WEBUI_UNREFERENCED_CANARY',
  'hami_webui_canary_should_not_ship'
]
const bundledForbiddenValue = forbiddenValues.find(contains)
if (bundledForbiddenValue) {
  throw new Error(
    `unreferenced build environment value was bundled: ${bundledForbiddenValue}`
  )
}

const requiredValues = ['/vite-env-contract/', '71234567']
const missingRequiredValue = requiredValues.find((value) => !contains(value))
if (missingRequiredValue) {
  throw new Error(
    `allowlisted Vite value was not bundled: ${missingRequiredValue}`
  )
}

console.log('Vite build environment boundary verified.')
