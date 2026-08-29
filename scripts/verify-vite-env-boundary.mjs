import { spawnSync } from 'node:child_process'
import assert from 'node:assert/strict'
import { readdir, readFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import axios from 'axios'

import { getBasePath } from '../packages/web/src/utils/base-path.mjs'

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
  HAMI_WEBUI_BACKEND_URL: 'http://private-backend-canary.invalid:8000',
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
  'http://private-backend-canary.invalid:8000',
  'HAMI_WEBUI_UNREFERENCED_CANARY',
  'hami_webui_canary_should_not_ship',
  '/vite-env-contract/'
]
const bundledForbiddenValue = forbiddenValues.find(contains)
if (bundledForbiddenValue) {
  throw new Error(
    `unreferenced build environment value was bundled: ${bundledForbiddenValue}`
  )
}

const requiredValues = ['71234567']
const missingRequiredValue = requiredValues.find((value) => !contains(value))
if (missingRequiredValue) {
  throw new Error(
    `allowlisted Vite value was not bundled: ${missingRequiredValue}`
  )
}

const builtIndex = await readFile(path.join(outputDirectory, 'index.html'), 'utf8')
if (
  !/<base\b(?=[^>]*\bdata-hami-webui-base\b)(?=[^>]*\bhref="\/")[^>]*>/i.test(
    builtIndex
  )
) {
  throw new Error('built index is missing the runtime base-path marker')
}
if (
  !/<link\b(?=[^>]*\brel="icon")(?=[^>]*\bhref="\.\/favicon\.svg")[^>]*>/i.test(
    builtIndex
  )
) {
  throw new Error('built index favicon is not relative to the runtime base path')
}
if (
  builtIndex.indexOf('data-hami-webui-base') >
  builtIndex.indexOf('href="./favicon.svg"')
) {
  throw new Error('runtime base-path marker must precede relative document URLs')
}

const basePathCases = [
  [undefined, '/'],
  ['https://hami.example.test/', '/'],
  ['https://hami.example.test/platform/hami/', '/platform/hami/'],
  ['https://hami.example.test/platform/hami', '/platform/hami/'],
  ['https://hami.example.test/platform/hami/?view=nodes#active', '/platform/hami/'],
  ['not a URL', '/']
]
for (const [baseURI, expected] of basePathCases) {
  assert.equal(getBasePath(baseURI), expected, String(baseURI))
}

for (const [baseURI, expected] of [
  ['https://hami.example.test/', '/api/vgpu/v1/nodes'],
  [
    'https://hami.example.test/platform/hami/',
    '/platform/hami/api/vgpu/v1/nodes'
  ]
]) {
  const client = axios.create({ baseURL: getBasePath(baseURI) })
  assert.equal(client.getUri({ url: '/api/vgpu/v1/nodes' }), expected)
}

console.log('Vite build environment boundary verified.')
