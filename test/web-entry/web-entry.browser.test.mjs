import assert from 'node:assert/strict'
import http from 'node:http'
import { after, before, test } from 'node:test'
import { setTimeout as delay } from 'node:timers/promises'

import { chromium } from 'playwright'

import { launchWebEntry } from './launch-web-entry.mjs'

const host = '127.0.0.1'
const basePath = '/gpu-ui/'
const deepRoute = `${basePath}admin/vgpu/monitor/overview`
const servers = []
const processes = []

let backend
let backendURL
let browser

function escapeAttribute(value) {
  return value.replaceAll('&', '&amp;').replaceAll('"', '&quot;')
}

async function listen(server, port = 0) {
  await new Promise((resolve, reject) => {
    const onError = (error) => reject(error)
    const onListening = () => {
      server.off('error', onError)
      resolve()
    }
    server.once('error', onError)
    server.listen(port, host, onListening)
  })
  servers.push(server)
  const address = server.address()
  assert.equal(typeof address, 'object')
  return `http://${host}:${address.port}`
}

async function closeServer(server) {
  if (!server?.listening) return
  const closed = new Promise((resolve) => server.close(resolve))
  server.closeAllConnections?.()
  await closed
}

async function stopProcess(child) {
  if (!child || child.exitCode !== null || child.signalCode !== null) return
  const exited = new Promise((resolve) => child.once('exit', resolve))
  child.kill('SIGTERM')
  await Promise.race([
    exited,
    delay(2_000).then(() => {
      child.kill('SIGKILL')
      return exited
    })
  ])
}

async function waitUntil(check, message, timeout = 8_000) {
  const deadline = Date.now() + timeout
  while (Date.now() < deadline) {
    const value = await check()
    if (value) return value
    await delay(50)
  }
  throw new Error(message)
}

async function startWebEntry({ frameAncestors }) {
  const probe = http.createServer()
  const origin = await listen(probe)
  const address = probe.address()
  await closeServer(probe)

  const logs = []
  const child = launchWebEntry({
    cwd: process.cwd(),
    listenAddress: `${host}:${address.port}`,
    backendURL,
    basePath,
    frameAncestors
  })
  processes.push(child)
  for (const stream of [child.stdout, child.stderr]) {
    stream.on('data', (chunk) => logs.push(String(chunk)))
  }

  await waitUntil(async() => {
    if (child.exitCode !== null || child.signalCode !== null) {
      throw new Error(`Web entry exited during startup:\n${logs.join('')}`)
    }
    try {
      const response = await fetch(`${origin}/health_check`, {
        signal: AbortSignal.timeout(300)
      })
      await response.arrayBuffer()
      return response.ok
    } catch {
      return false
    }
  }, `Timed out starting Web entry:\n${logs.join('')}`)

  return origin
}

function proxyRequest(req, res, targetOrigin) {
  const target = new URL(req.url, targetOrigin)
  const upstream = http.request(target, {
    method: req.method,
    headers: req.headers
  }, (upstreamResponse) => {
    res.writeHead(upstreamResponse.statusCode ?? 502, upstreamResponse.headers)
    upstreamResponse.pipe(res)
  })
  upstream.on('error', () => {
    res.writeHead(502)
    res.end('proxy failure')
  })
  req.pipe(upstream)
}

async function startParent({ iframeURL, proxyTarget, nestedParentURL }) {
  return listen(http.createServer((req, res) => {
    if (req.url === '/parent') {
      const source = nestedParentURL ?? iframeURL
      res.writeHead(200, {
        'content-type': 'text/html; charset=utf-8',
        'cache-control': 'no-store'
      })
      res.end(`<!doctype html><title>Embedding host</title><iframe id="embedded" src="${escapeAttribute(source)}"></iframe>`)
      return
    }
    if (proxyTarget) {
      proxyRequest(req, res, proxyTarget)
      return
    }
    res.writeHead(404)
    res.end('not found')
  }))
}

async function loadAllowedFrame(parentURL, expectedFrameURL, { checkAPI = false } = {}) {
  const page = await browser.newPage()
  const failedAssets = []
  const apiRequests = []
  page.on('request', (request) => {
    if (request.url().includes(`${basePath}api/vgpu/`)) {
      apiRequests.push(request.url())
    }
  })
  page.on('requestfailed', (request) => {
    if (['script', 'stylesheet', 'image'].includes(request.resourceType())) {
      failedAssets.push(`${request.resourceType()}: ${request.url()}`)
    }
  })
  page.on('response', (response) => {
    if (
      ['script', 'stylesheet', 'image'].includes(response.request().resourceType()) &&
      response.status() >= 400
    ) {
      failedAssets.push(`${response.status()}: ${response.url()}`)
    }
  })

  await page.goto(`${parentURL}/parent`, { waitUntil: 'domcontentloaded' })
  const frame = await waitUntil(
    () => page.frames().find((candidate) => candidate.url() === expectedFrameURL),
    `Expected iframe navigation to ${expectedFrameURL}`
  )
  await frame.waitForSelector('#app')
  await frame.waitForFunction(() => document.querySelector('#app')?.childElementCount > 0)
  assert.equal(await frame.evaluate(() => new URL(document.baseURI).pathname), basePath)
  if (checkAPI) {
    await waitUntil(() => apiRequests.length > 0, 'SPA did not issue a base-prefixed API request')
  }
  assert.deepEqual(failedAssets, [])
  await page.close()
}

async function assertFrameBlocked(parentURL, targetOrigin) {
  const page = await browser.newPage()
  const targetResponse = page.waitForResponse((response) =>
    response.request().resourceType() === 'document' &&
    response.url().startsWith(`${targetOrigin}${basePath}`)
  )
  await page.goto(`${parentURL}/parent`, { waitUntil: 'domcontentloaded' })
  const response = await targetResponse
  assert.equal(response.status(), 200)
  // Let Chromium apply the framing directive after receiving the document.
  await delay(100)
  assert.equal(
    page.frames().some((frame) => frame.url().startsWith(`${targetOrigin}${basePath}`)),
    false,
    `Browser committed a frame that CSP should block: ${targetOrigin}`
  )
  await page.close()
}

before(async() => {
  backend = http.createServer((req, res) => {
    req.resume()
    res.writeHead(200, { 'content-type': 'application/json' })
    res.end(JSON.stringify({ code: 0, data: {}, list: [], total: 0 }))
  })
  backendURL = await listen(backend)
  browser = await chromium.launch({ headless: true })
}, { timeout: 30_000 })

after(async() => {
  await browser?.close()
  await Promise.all(processes.map(stopProcess))
  await Promise.all(servers.map(closeServer))
}, { timeout: 30_000 })

test('runtime base path and framing policy work in Chromium', async(t) => {
  await t.test('unset policy preserves cross-origin embedding', async() => {
    const target = await startWebEntry({ frameAncestors: undefined })
    const parent = await startParent({ iframeURL: `${target}${deepRoute}` })
    await loadAllowedFrame(parent, `${target}${deepRoute}`, { checkAPI: true })

    const page = await browser.newPage()
    await page.goto(`${target}${basePath.slice(0, -1)}`, { waitUntil: 'domcontentloaded' })
    await page.waitForSelector('#app')
    await page.waitForFunction(() => document.querySelector('#app')?.childElementCount > 0)
    assert.equal(await page.evaluate(() => new URL(document.baseURI).pathname), basePath)
    await page.close()
  })

  await t.test('empty policy blocks frames but not top-level navigation', async() => {
    const target = await startWebEntry({ frameAncestors: [] })
    const page = await browser.newPage()
    await page.goto(`${target}${deepRoute}`, { waitUntil: 'domcontentloaded' })
    await page.waitForSelector('#app')
    assert.equal(new URL(page.url()).pathname, deepRoute)
    await page.close()

    const parent = await startParent({ iframeURL: `${target}${deepRoute}` })
    await assertFrameBlocked(parent, target)
  })

  await t.test("'self' allows same-origin and rejects cross-origin parents", async() => {
    const target = await startWebEntry({ frameAncestors: ["'self'"] })
    const sameOriginParent = await startParent({
      iframeURL: deepRoute,
      proxyTarget: target
    })
    await loadAllowedFrame(sameOriginParent, `${sameOriginParent}${deepRoute}`)

    const crossOriginParent = await startParent({ iframeURL: `${target}${deepRoute}` })
    await assertFrameBlocked(crossOriginParent, target)
  })

  await t.test('explicit origin allows only the complete ancestor chain', async() => {
    const parentPortProbe = http.createServer()
    const allowedParent = await listen(parentPortProbe)
    const parentPort = parentPortProbe.address().port
    await closeServer(parentPortProbe)

    const target = await startWebEntry({ frameAncestors: [allowedParent] })
    const allowedParentServer = http.createServer((req, res) => {
      if (req.url !== '/parent') {
        res.writeHead(404)
        res.end('not found')
        return
      }
      res.writeHead(200, { 'content-type': 'text/html; charset=utf-8' })
      res.end(`<!doctype html><iframe id="embedded" src="${target}${deepRoute}"></iframe>`)
    })
    await listen(allowedParentServer, parentPort)
    await loadAllowedFrame(allowedParent, `${target}${deepRoute}`)

    const unlistedParent = await startParent({ iframeURL: `${target}${deepRoute}` })
    await assertFrameBlocked(unlistedParent, target)

    const topParent = await startParent({
      nestedParentURL: `${allowedParent}/parent`
    })
    await assertFrameBlocked(topParent, target)
  })
}, { timeout: 120_000 })
