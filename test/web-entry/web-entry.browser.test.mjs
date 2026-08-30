import assert from 'node:assert/strict'
import http from 'node:http'
import { after, before, test } from 'node:test'
import { setTimeout as delay } from 'node:timers/promises'

import { chromium } from 'playwright'

import { expectedIconIds } from '../../packages/web/test/expected-icon-catalog.mjs'
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
  const svgSprite = await frame.evaluate(() => {
    const root = document.getElementById('__svg__icons__dom__')
    const symbolIDs = Array.from(
      root?.querySelectorAll('symbol') ?? [],
      (symbol) => symbol.id
    )
    const renderedUse = Array.from(document.querySelectorAll('use')).find(
      (use) =>
        (use.getAttribute('href') || use.getAttribute('xlink:href')) ===
        '#icon-more'
    )
    const renderedBox = renderedUse?.getBBox()
    const referencedSymbol = document.getElementById('icon-more')
    return {
      present: Boolean(root),
      referencedSymbolTag: referencedSymbol?.tagName.toLowerCase(),
      renderedHeight: renderedBox?.height ?? 0,
      renderedWidth: renderedBox?.width ?? 0,
      symbolIDs,
      symbolCount: symbolIDs.length,
      uniqueSymbolCount: new Set(symbolIDs).size
    }
  })
  assert.equal(svgSprite.present, true, 'SVG sprite root was not registered')
  assert.deepEqual(
    [...svgSprite.symbolIDs].sort(),
    [...expectedIconIds].sort(),
    'SVG sprite does not match the retained icon catalog'
  )
  assert.equal(
    svgSprite.uniqueSymbolCount,
    svgSprite.symbolCount,
    'SVG sprite contains duplicate symbol IDs'
  )
  assert.equal(svgSprite.referencedSymbolTag, 'symbol')
  assert.ok(svgSprite.renderedWidth > 0, 'Rendered SVG use has no width')
  assert.ok(svgSprite.renderedHeight > 0, 'Rendered SVG use has no height')
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

async function assertChartRuntime(target) {
  const page = await browser.newPage({
    viewport: { width: 1440, height: 1000 }
  })
  const runtimeErrors = []
  let rangeRequests = 0
  page.on('pageerror', (error) => runtimeErrors.push(error.message))
  page.on('request', (request) => {
    if (request.url().includes('/v1/monitor/query/range-vector')) {
      rangeRequests += 1
    }
  })
  page.on('console', (message) => {
    if (
      ['warning', 'error'].includes(message.type()) &&
      message.text().includes('[ECharts]')
    ) {
      runtimeErrors.push(message.text())
    }
  })

  await page.goto(`${target}${deepRoute}`, { waitUntil: 'domcontentloaded' })
  await page.waitForFunction(
    () => document.querySelectorAll('.echarts canvas').length >= 4
  )
  await page.waitForFunction(() =>
    [...document.querySelectorAll('.tab-top-value')]
      .some((element) => element.textContent?.trim() === '0.4 %')
  )

  const initialRangeRequests = rangeRequests
  await page.locator('.home-bottom-trend-filter .t-radio-button').nth(1).click()
  await waitUntil(
    () => rangeRequests > initialRangeRequests,
    'Changing the time range did not update the chart data'
  )
  assert.ok(
    await page.locator('.home-bottom-row .echarts canvas').count() >= 2,
    'Trend charts disappeared after an option update'
  )

  const pie = page.locator('.card-type-chart canvas').first()
  const pieBox = await pie.boundingBox()
  assert.ok(pieBox?.width > 0, 'Overview card-type chart has no width')
  assert.ok(pieBox?.height > 0, 'Overview card-type chart has no height')
  await pie.click({
    position: { x: pieBox.width * 0.75, y: pieBox.height * 0.5 }
  })
  await page.waitForURL((url) =>
    url.pathname.endsWith('/admin/vgpu/card/admin') &&
    url.searchParams.get('type') === 'NVIDIA'
  )

  const previewChart = page.locator('.pie .echarts canvas').first()
  await previewChart.waitFor()
  await page.setViewportSize({ width: 1024, height: 768 })
  await waitUntil(async() => {
    const box = await previewChart.boundingBox()
    return box && box.width > 0 && box.height > 0
  }, 'Preview chart did not survive a resize')
  await page.waitForFunction(() => {
    const canvas = document.querySelector('.pie .echarts canvas')
    const context = canvas?.getContext('2d')
    if (!canvas?.width || !canvas.height || !context) return false
    const pixels = context.getImageData(0, 0, canvas.width, canvas.height).data
    for (let index = 0; index < pixels.length; index += 4) {
      const red = pixels[index]
      const green = pixels[index + 1]
      const blue = pixels[index + 2]
      const alpha = pixels[index + 3]
      if (alpha > 0 && green > 100 && green > red * 1.2 && green > blue * 1.2) {
        return true
      }
    }
    return false
  })
  await waitUntil(async() => previewChart.evaluate(async(canvas) => {
    const frame = canvas.toDataURL()
    await new Promise((resolve) => setTimeout(resolve, 100))
    return frame === canvas.toDataURL()
  }), 'Preview chart animation did not settle')
  const previewLegendItem = page.locator('.nodeCard-legend li').first()
  assert.equal(
    await previewLegendItem.evaluate((element) => element.style.fontWeight),
    'bold'
  )
  const previewBox = await previewChart.boundingBox()
  assert.ok(previewBox?.width > 0, 'Preview chart has no width after resize')
  assert.ok(previewBox?.height > 0, 'Preview chart has no height after resize')
  const paintedSlice = await previewChart.evaluate((canvas) => {
    const context = canvas.getContext('2d')
    const pixels = context.getImageData(0, 0, canvas.width, canvas.height).data
    let longestRun = 0
    let point = null

    for (let y = 0; y < canvas.height; y += 1) {
      let runStart = -1
      for (let x = 0; x < canvas.width; x += 1) {
        const index = (y * canvas.width + x) * 4
        const red = pixels[index]
        const green = pixels[index + 1]
        const blue = pixels[index + 2]
        const alpha = pixels[index + 3]
        const isGreen = alpha > 0 && green > 100 && green > red * 1.2 && green > blue * 1.2

        if (isGreen && runStart === -1) runStart = x
        if ((!isGreen || x === canvas.width - 1) && runStart !== -1) {
          const runEnd = isGreen ? x : x - 1
          const runLength = runEnd - runStart + 1
          if (runLength > longestRun) {
            longestRun = runLength
            point = { x: (runStart + runEnd) / 2, y }
          }
          runStart = -1
        }
      }
    }

    return point
      ? { ...point, width: canvas.width, height: canvas.height }
      : null
  })
  assert.ok(paintedSlice, 'Preview chart did not paint the active slice')
  await previewChart.click({
    position: {
      x: paintedSlice.x / paintedSlice.width * previewBox.width,
      y: paintedSlice.y / paintedSlice.height * previewBox.height
    }
  })
  await waitUntil(
    () => previewLegendItem.evaluate(
      (element) => element.style.fontWeight === 'normal'
    ),
    'Preview chart click did not clear the active card-type filter'
  )

  assert.deepEqual(runtimeErrors, [])
  await page.close()
}

before(async() => {
  backend = http.createServer((req, res) => {
    req.resume()
    const pathname = new URL(req.url, 'http://backend.local').pathname
    const now = Math.floor(Date.now() / 1000)
    let payload = { code: 0, data: {}, list: [], total: 0 }

    if (pathname === '/v1/nodes') {
      payload = {
        code: 0,
        list: [{ name: 'node-1', uid: 'node-1', isExternal: false, isSchedulable: true }],
        total: 1
      }
    } else if (pathname === '/v1/gpus') {
      payload = {
        code: 0,
        list: [{ uuid: 'gpu-1', type: 'NVIDIA', node: 'node-1', health: true }],
        total: 1
      }
    } else if (pathname === '/v1/containers') {
      payload = { code: 0, items: [{ name: 'worker', podName: 'job-1' }], total: 1 }
    } else if (pathname === '/v1/monitor/query/instant-vector') {
      payload = {
        code: 0,
        data: [{
          metric: {
            device_type: 'NVIDIA',
            device_uuid: 'gpu-1',
            node: 'node-1',
            provider: 'NVIDIA'
          },
          value: 0.4
        }]
      }
    } else if (pathname === '/v1/monitor/query/range-vector') {
      payload = {
        code: 0,
        data: [{
          metric: { node: 'node-1' },
          values: [[now - 60, 20], [now, 42]]
        }]
      }
    }

    res.writeHead(200, { 'content-type': 'application/json' })
    res.end(JSON.stringify(payload))
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

test('ECharts runtime renders, updates and handles interaction in Chromium', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  await assertChartRuntime(target)
}, { timeout: 60_000 })
