import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import http from 'node:http'
import net from 'node:net'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { after, before, test } from 'node:test'
import { setTimeout as delay } from 'node:timers/promises'

import { chromium } from 'playwright'

const cwd = process.cwd()
const host = '127.0.0.1'
const basePath = '/gpu-ui/'
const watchStreams = new Set()
const servers = []

let tempDir
let k8sServer
let promServer
let parentServer
let browser
let childProc
let backendPort
let webPort
let parentOrigin
let promHits = 0

function freePort() {
  return new Promise((resolve, reject) => {
    const srv = net.createServer()
    srv.once('error', reject)
    srv.listen(0, host, () => {
      const addr = srv.address()
      const port = typeof addr === 'object' && addr ? addr.port : 0
      srv.close((err) => (err ? reject(err) : resolve(port)))
    })
  })
}

function listen(server) {
  return new Promise((resolve, reject) => {
    const onError = (err) => reject(err)
    server.once('error', onError)
    server.listen(0, host, () => {
      server.off('error', onError)
      servers.push(server)
      const addr = server.address()
      assert.equal(typeof addr, 'object')
      resolve(`http://${host}:${addr.port}`)
    })
  })
}

async function waitHTTP200(url, timeoutMs = 8000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url)
      await res.arrayBuffer()
      if (res.status === 200) return
    } catch {
      // retry until server is listening
    }
    await delay(50)
  }
  throw new Error(`Timed out waiting for 200 from ${url}`)
}

async function stopProcess(proc) {
  if (!proc || proc.exitCode !== null || proc.signalCode !== null) return
  const exited = new Promise((resolve) => proc.once('exit', resolve))
  proc.kill('SIGTERM')
  const winner = await Promise.race([exited.then(() => 'exited'), delay(3000).then(() => 'timeout')])
  if (winner === 'timeout') {
    proc.kill('SIGKILL')
    await exited
  }
}

before(async () => {
  tempDir = await mkdtemp(path.join(tmpdir(), 'hami-unified-smoke-'))
  const nodeFixture = {
    metadata: {
      name: 'node-gpu-01',
      uid: 'node-uid-01',
      resourceVersion: '10',
      labels: { gpu: 'on' },
      annotations: {
        'hami.io/node-nvidia-register': 'GPU-smoke-01,10,81920,100,NVIDIA-A100,0,true:',
      },
    },
    status: {
      conditions: [{ type: 'Ready', status: 'True' }],
      addresses: [{ type: 'InternalIP', address: '10.0.0.10' }],
    },
  }
  const podFixture = {
    metadata: {
      namespace: 'ns-smoke',
      name: 'train-pod-0',
      uid: 'pod-uid-01',
      resourceVersion: '10',
      creationTimestamp: '2026-11-26T00:00:00Z',
      annotations: {
        'hami.io/vgpu-node': 'node-gpu-01',
        'hami.io/vgpu-devices-allocated': 'GPU-smoke-01,NVIDIA,16384,50:;',
        'hami.io/vgpu-time': '1700000000',
      },
    },
    spec: {
      nodeName: 'node-gpu-01',
      containers: [
        {
          name: 'trainer',
          resources: { limits: { 'nvidia.com/gpu': '1' } },
        },
      ],
    },
    status: { phase: 'Running', podIP: '10.244.0.9' },
  }

  k8sServer = http.createServer((req, res) => {
    const reqUrl = new URL(req.url, 'http://127.0.0.1')
    const isWatch = reqUrl.searchParams.get('watch') === 'true' || reqUrl.searchParams.get('watch') === '1'
    const writeList = (kind, items) => {
      res.setHeader('Content-Type', 'application/json')
      res.writeHead(200)
      if (isWatch) {
        res.flushHeaders()
        watchStreams.add(res)
        req.on('close', () => watchStreams.delete(res))
        return
      }
      res.end(JSON.stringify({ apiVersion: 'v1', kind, metadata: { resourceVersion: '10' }, items }))
    }
    if (reqUrl.pathname === '/api/v1/nodes') return writeList('NodeList', [nodeFixture])
    if (reqUrl.pathname === '/api/v1/pods') return writeList('PodList', [podFixture])
    if (reqUrl.pathname === '/api/v1/events') return writeList('EventList', [])
    res.writeHead(404)
    res.end()
  })
  const k8sURL = await listen(k8sServer)

  promServer = http.createServer((_req, res) => {
    promHits += 1
    res.writeHead(503, { 'Content-Type': 'text/plain' })
    res.end('prometheus unavailable')
  })
  const promURL = await listen(promServer)

  parentServer = http.createServer((_req, res) => {
    res.setHeader('Content-Type', 'text/html; charset=utf-8')
    res.end(`<!doctype html><html><body><iframe id="webui-frame" src="http://${host}:${webPort}${basePath}" style="width:1280px;height:800px;border:0"></iframe></body></html>`)
  })
  parentOrigin = await listen(parentServer)

  backendPort = await freePort()
  webPort = await freePort()
  const grpcPort = await freePort()

  const kubeconfigPath = path.join(tempDir, 'kubeconfig')
  await writeFile(
    kubeconfigPath,
    `apiVersion: v1\nkind: Config\nclusters:\n- name: mock\n  cluster:\n    server: ${k8sURL}\ncontexts:\n- name: mock\n  context:\n    cluster: mock\n    user: mock\ncurrent-context: mock\nusers:\n- name: mock\n  user:\n    token: smoke-token\n`,
  )

  const configPath = path.join(tempDir, 'config.yaml')
  await writeFile(
    configPath,
    `server:\n  http:\n    addr: ${host}:${backendPort}\n    timeout: 5s\n  grpc:\n    addr: ${host}:${grpcPort}\n    timeout: 5s\nprometheus:\n  address: ${promURL}\n  timeout: 50ms\nexporter:\n  interval: 0.5s\n  timeout: 1.5s\ndevice_config:\n  enabled: false\nnode_selectors:\n  NVIDIA: gpu=on\n`,
  )

  const binaryPath = path.join(cwd, 'server', 'build', 'hami-webui')
  childProc = spawn(
    binaryPath,
    [
      `--conf=${configPath}`,
      `--listen-address=${host}:${webPort}`,
      `--static-dir=${path.join(cwd, 'public')}`,
      `--base-path=${basePath}`,
      `--frame-ancestors-json=${JSON.stringify(["'self'", parentOrigin])}`,
    ],
    {
      cwd,
      env: { ...process.env, KUBECONFIG: kubeconfigPath, TZ: 'UTC' },
      stdio: ['ignore', 'pipe', 'pipe'],
    },
  )

  await waitHTTP200(`http://${host}:${backendPort}/readyz`)
  await waitHTTP200(`http://${host}:${webPort}/health_check`)

  const watchDeadline = Date.now() + 4000
  let complete = false
  while (Date.now() < watchDeadline) {
    const res = await fetch(`http://${host}:${webPort}${basePath}api/v1/overview/inventory`)
    if (res.status === 200) {
      const json = await res.json()
      if (json?.nodeCoverage?.complete === true && json?.deviceCoverage?.complete === true) {
        complete = true
        break
      }
    }
    await delay(50)
  }
  assert.equal(complete, true, 'expected K8s WATCH continuity to report complete inventory coverage')

  browser = await chromium.launch({ headless: true })
})

after(async () => {
  await browser?.close()
  await stopProcess(childProc)
  for (const res of watchStreams) res.end()
  watchStreams.clear()
  for (const srv of servers) {
    srv.closeAllConnections?.()
    await new Promise((resolve) => srv.close(resolve))
  }
  if (tempDir) await rm(tempDir, { recursive: true, force: true })
})

test('unified hami-webui binary serves real K8s inventory, /metrics, and subpath UI under Prometheus 503', async () => {
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } })
  const page = await context.newPage()
  const pageErrors = []
  page.on('pageerror', (err) => pageErrors.push(err))

  try {
    const inventoryPromise = page.waitForResponse((res) => res.url().endsWith('/api/v1/overview/inventory'))
    await page.goto(`http://${host}:${webPort}${basePath}`, { waitUntil: 'domcontentloaded' })
    const invRes = await inventoryPromise
    assert.equal(invRes.status(), 200)
    const invJson = await invRes.json()
    assert.equal(invJson.nodeCoverage?.complete, true)
    assert.equal(invJson.deviceCoverage?.complete, true)
    assert.equal(invJson.nodes?.[0]?.name, 'node-gpu-01')
    assert.equal(invJson.devices?.[0]?.deviceId, 'GPU-smoke-01')
    assert.equal(invJson.devices?.[0]?.utilizationRatio, undefined)

    await page.waitForSelector('a[href*="/nodes"]')
    await page.click('a[href*="/nodes"]')
    await page.waitForFunction(() => document.body.innerText.includes('node-gpu-01'))

    await page.click('a[href*="/accelerators"]')
    await page.waitForFunction(() => document.body.innerText.includes('GPU-smoke-01'))

    await page.click('a[href*="/workloads"]')
    await page.waitForFunction(() => document.body.innerText.includes('train-pod-0'))

    const metricsDeadline = Date.now() + 6000
    let metricsText = ''
    while (Date.now() < metricsDeadline) {
      const metricsRes = await fetch(`http://${host}:${backendPort}/metrics`)
      assert.equal(metricsRes.status, 200)
      metricsText = await metricsRes.text()
      if (/^hami_vgpu_count\{[^}]*device_uuid="GPU-smoke-01"[^}]*\}\s+10$/m.test(metricsText)) {
        break
      }
      await delay(50)
    }
    assert.match(metricsText, /^hami_vgpu_count\{[^}]*device_uuid="GPU-smoke-01"[^}]*\}\s+10$/m)
    assert.ok(promHits > 0)
    assert.deepEqual(pageErrors, [])
  } finally {
    await context.close()
  }
})

test('unified hami-webui binary renders and navigates inside an iframe when localStorage throws SecurityError', async () => {
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } })
  await context.addInitScript(() => {
    if (window !== window.top) {
      Object.defineProperty(window, 'localStorage', {
        configurable: true,
        get() {
          throw new DOMException('localStorage disabled in sandboxed iframe', 'SecurityError')
        },
      })
    }
  })

  const page = await context.newPage()
  const pageErrors = []
  page.on('pageerror', (err) => pageErrors.push(err))

  try {
    await page.goto(parentOrigin, { waitUntil: 'domcontentloaded' })
    const frame = page.frameLocator('#webui-frame')
    await frame.locator('a.side-link[href*="/nodes"]').waitFor({ timeout: 10000 })
    await frame.locator('a.side-link[href*="/nodes"]').click()
    await frame.locator('text=node-gpu-01').first().waitFor({ timeout: 10000 })
    await frame.locator('a.side-link[href*="/accelerators"]').click()
    await frame.locator('text=GPU-smoke-01').first().waitFor({ timeout: 10000 })
    assert.deepEqual(pageErrors, [])
  } finally {
    await context.close()
  }
})
