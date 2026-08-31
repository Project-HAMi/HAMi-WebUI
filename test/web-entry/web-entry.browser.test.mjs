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
const backendRequestCounts = new Map()
const responseGates = new Map()

let backend
let backendURL
let browser

function countBackendRequest(key) {
  const count = (backendRequestCounts.get(key) ?? 0) + 1
  backendRequestCounts.set(key, count)
  return count
}

function createResponseGate(key) {
  let release
  const promise = new Promise((resolve) => {
    release = resolve
  })
  const gate = { promise, release }
  responseGates.set(key, gate)
  return gate
}

async function waitForResponseGate(key) {
  await responseGates.get(key)?.promise
}

async function readJSONBody(req) {
  const chunks = []
  for await (const chunk of req) chunks.push(chunk)
  if (chunks.length === 0) return undefined
  try {
    return JSON.parse(Buffer.concat(chunks).toString('utf8'))
  } catch {
    return undefined
  }
}

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

function trackMonitorRequests(page) {
  const requests = []
  page.on('request', (request) => {
    if (request.url().includes('/v1/monitor/query/')) {
      requests.push(request.url())
    }
  })
  return requests
}

async function assertMissingDetail(target, route) {
  const page = await browser.newPage()
  const monitorRequests = trackMonitorRequests(page)
  try {
    await page.goto(`${target}${route}`, { waitUntil: 'domcontentloaded' })
    await page.locator('[data-testid="detail-page-missing"]').waitFor()
    assert.equal(
      await page.locator('[data-testid="detail-page-error"]').count(),
      0
    )
    await delay(100)
    assert.equal(
      monitorRequests.length,
      0,
      `Missing detail issued monitor requests: ${route}`
    )
  } finally {
    await page.close()
  }
}

before(async() => {
  backend = http.createServer(async(req, res) => {
    const body = await readJSONBody(req)
    const requestURL = new URL(req.url, 'http://backend.local')
    const { pathname } = requestURL
    const now = Math.floor(Date.now() / 1000)
    let payload = { code: 0, data: {}, list: [], total: 0 }

    if (pathname === '/v1/gpu') {
      const uid = requestURL.searchParams.get('uid') ?? ''
      const requestKey = `gpu:${uid}`
      countBackendRequest(requestKey)
      await waitForResponseGate(requestKey)
      payload = uid === 'gpu-missing'
        ? {
            uuid: '',
            nodeName: '',
            type: '',
            vgpuUsed: 0,
            vgpuTotal: 0,
            coreUsed: 0,
            coreTotal: 0,
            memoryUsed: 0,
            memoryTotal: 0,
            nodeUid: '',
            health: false,
            mode: ''
          }
        : {
            uuid: uid,
            nodeName: 'node-1',
            type: 'NVIDIA',
            vgpuUsed: 0,
            vgpuTotal: 1,
            coreUsed: 0,
            coreTotal: 100,
            memoryUsed: 0,
            memoryTotal: 16_384,
            nodeUid: 'node-1',
            health: true,
            mode: ''
          }
    } else if (pathname === '/v1/node') {
      const uid = requestURL.searchParams.get('uid') ?? ''
      const requestKey = `node:${uid}`
      const attempt = countBackendRequest(requestKey)
      if (uid === 'node-retry' && attempt === 1) {
        res.writeHead(503, { 'content-type': 'application/json' })
        res.end(JSON.stringify({
          code: 503,
          reason: 'TEMPORARILY_UNAVAILABLE',
          message: 'temporary node lookup failure'
        }))
        return
      }
      payload = {
        uid,
        name: uid,
        ip: '192.0.2.10',
        isSchedulable: true,
        isReady: true,
        type: ['NVIDIA'],
        vgpuUsed: 0,
        vgpuTotal: 1,
        coreUsed: 0,
        coreTotal: 100,
        memoryUsed: 0,
        memoryTotal: 16_384,
        cardCnt: 1
      }
    } else if (pathname === '/v1/container') {
      const name = requestURL.searchParams.get('name') ?? ''
      const podUid = requestURL.searchParams.get('podUid') ?? ''
      countBackendRequest(`container:${podUid}:${name}`)
      payload = name === 'missing-worker'
        ? {
            name: '',
            status: '',
            appName: '',
            nodeName: '',
            allocatedDevices: 0,
            allocatedCores: 0,
            allocatedMem: 0,
            type: '',
            createTime: '',
            startTime: '',
            endTime: '',
            podUid: '',
            nodeUid: '',
            resourcePool: '',
            flavor: '',
            priority: '',
            namespace: '',
            deviceIds: [],
            images: []
          }
        : {
            name,
            status: 'success',
            appName: 'job-1',
            nodeName: 'node-1',
            allocatedDevices: 1,
            allocatedCores: 100,
            allocatedMem: 1024,
            type: 'NVIDIA',
            podUid,
            nodeUid: 'node-1',
            namespace: 'default',
            deviceIds: ['gpu-1'],
            images: ['example.invalid/worker:latest']
          }
    } else if (pathname === '/v1/nodes') {
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
      if (body?.query?.includes('gpu-metric-new')) {
        const requestKey = 'monitor:gpu-metric-new'
        countBackendRequest(requestKey)
        await waitForResponseGate(requestKey)
      }
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
      if (body?.query?.includes('gpu-metric-new')) {
        const requestKey = 'monitor:gpu-metric-new'
        countBackendRequest(requestKey)
        await waitForResponseGate(requestKey)
      }
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

test('unknown routes render a local, responsive HAMi page', async(t) => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const unknownRoute = `${basePath}missing/reports/does-not-exist?source=browser#summary`

  await t.test('the top-level page retains its URL and language controls', async() => {
    const requestedURLs = []
    const page = await browser.newPage({
      locale: 'en-US',
      viewport: { width: 375, height: 667 }
    })
    page.on('request', (request) => requestedURLs.push(request.url()))

    try {
      await page.goto(`${target}${unknownRoute}`, { waitUntil: 'domcontentloaded' })
      await page.locator('[data-testid="not-found-page"]').waitFor()
      await page.getByRole('heading', { name: 'Page not found' }).waitFor()

      const currentURL = new URL(page.url())
      assert.equal(
        `${currentURL.pathname}${currentURL.search}${currentURL.hash}`,
        unknownRoute
      )

      const overviewLink = page.locator('[data-testid="not-found-overview-link"]')
      const overviewURL = new URL(await overviewLink.getAttribute('href'), target)
      assert.equal(overviewURL.origin, new URL(target).origin)
      assert.equal(overviewURL.pathname, deepRoute)

      const dimensions = await page.evaluate(() => ({
        body: document.body.scrollWidth,
        document: document.documentElement.scrollWidth,
        viewport: window.innerWidth
      }))
      assert.ok(
        dimensions.body <= dimensions.viewport,
        `404 body overflows horizontally: ${JSON.stringify(dimensions)}`
      )
      assert.ok(
        dimensions.document <= dimensions.viewport,
        `404 document overflows horizontally: ${JSON.stringify(dimensions)}`
      )

      const targetOrigin = new URL(target).origin
      const externalRequests = requestedURLs.filter((requestURL) => {
        const parsed = new URL(requestURL)
        return ['http:', 'https:'].includes(parsed.protocol) && parsed.origin !== targetOrigin
      })
      assert.deepEqual(externalRequests, [])
      assert.equal(requestedURLs.some((url) => url.includes('wallstcn')), false)

      await page.locator('.lang-select-container').click()
      await page.locator('.lang-dropdown-popper .el-dropdown-menu__item')
        .filter({ hasText: '中文' })
        .click()
      await page.locator('html[lang="zh-CN"]').waitFor()
      await page.getByRole('heading', { name: '页面未找到' }).waitFor()
      assert.equal(page.url(), `${target}${unknownRoute}`)

      await page.locator('.lang-select-container').click()
      await page.locator('.lang-dropdown-popper .el-dropdown-menu__item')
        .filter({ hasText: 'English' })
        .click()
      await page.locator('html[lang="en"]').waitFor()
      await page.getByRole('heading', { name: 'Page not found' }).waitFor()
    } finally {
      await page.close()
    }
  })

  await t.test('the base-prefixed overview link works inside an iframe', async() => {
    const parent = await startParent({ iframeURL: `${target}${unknownRoute}` })
    const page = await browser.newPage({ locale: 'en-US' })

    try {
      await page.goto(`${parent}/parent`, { waitUntil: 'domcontentloaded' })
      const frame = await waitUntil(
        () => page.frames().find((candidate) => candidate.url() === `${target}${unknownRoute}`),
        `Expected iframe navigation to ${target}${unknownRoute}`
      )
      const overviewLink = frame.locator('[data-testid="not-found-overview-link"]')
      await overviewLink.waitFor()
      assert.equal(
        new URL(await overviewLink.getAttribute('href'), target).pathname,
        deepRoute
      )

      await Promise.all([
        frame.waitForURL((url) => url.pathname === deepRoute),
        overviewLink.click()
      ])
      await frame.locator('.home-page-title').waitFor()
    } finally {
      await page.close()
    }
  })
}, { timeout: 60_000 })

test('browser language selects English without leaking active Chinese UI text', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US' })

  try {
    await page.goto(
      `${target}${basePath}admin/vgpu/task/admin`,
      { waitUntil: 'domcontentloaded' }
    )
    await page.locator('.lang-text').filter({ hasText: 'English' }).waitFor()
    await page.locator('.workload-table .vgpu-table-name-text-wrap .ellipsis-text')
      .filter({ hasText: 'worker' })
      .waitFor()

    const requestsCard = page.locator('.task-top-box .home-block').nth(1)
    await requestsCard.locator('.title')
      .filter({ hasText: 'Workload Requests Top5' })
      .waitFor()
    await requestsCard.locator('.t-radio-button')
      .filter({ hasText: 'vGPU' })
      .click()
    const value = requestsCard.locator('.tab-top-value').first()
    await value.waitFor()
    assert.equal((await value.textContent()).trim(), '0.4 slots')

    await page.goto(`${target}${basePath}401`, { waitUntil: 'domcontentloaded' })
    await page.getByRole('heading', { name: 'Page not found' }).waitFor()
    assert.equal((await page.locator('body').textContent()).includes('页面未找到'), false)
  } finally {
    await page.close()
  }
}, { timeout: 60_000 })

test('runtime language updates the document and Element Plus services', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US' })
  let failNextNodesRequest = true

  await page.route('**/api/vgpu/v1/nodes**', (route) => {
    if (!failNextNodesRequest) return route.continue()
    failNextNodesRequest = false
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        code: 50008,
        message: 'authentication expired'
      })
    })
  })

  try {
    await page.goto(
      `${target}${basePath}admin/vgpu/node/admin`,
      { waitUntil: 'domcontentloaded' }
    )
    await page.locator('html[lang="en"]').waitFor()

    const messageBox = page.locator('.el-message-box')
    await messageBox.waitFor()
    await messageBox.locator('.el-button--primary')
      .filter({ hasText: 'OK' })
      .click()

    await page.locator('.lang-select-container').click()
    await page.locator('.lang-dropdown-popper .el-dropdown-menu__item')
      .filter({ hasText: '中文' })
      .click()
    await page.locator('html[lang="zh-CN"]').waitFor()

    failNextNodesRequest = true
    await page.reload({ waitUntil: 'domcontentloaded' })
    await page.locator('html[lang="zh-CN"]').waitFor()
    await messageBox.waitFor()
    await messageBox.locator('.el-button--primary')
      .filter({ hasText: '确定' })
      .click()

    await page.locator('.lang-select-container').click()
    await page.locator('.lang-dropdown-popper .el-dropdown-menu__item')
      .filter({ hasText: 'English' })
      .click()
    await page.locator('html[lang="en"]').waitFor()
  } finally {
    await page.close()
  }
}, { timeout: 60_000 })

test('workload list exposes deterministic loading, empty, error and refresh states', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US' })
  const responses = []
  let receivedRequests = 0
  let completedRequests = 0

  const enqueue = (handler) => responses.push(handler)
  const fulfill = (route, items, status = 200) => route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify(status === 200
      ? { code: 0, items, total: Array.isArray(items) ? items.length : 0 }
      : { code: status, message: 'temporary list failure' })
  })
  const workload = (name) => ({
    name,
    appName: '',
    podUid: `pod-${name}`,
    namespace: 'default',
    status: 'success',
    deviceIds: ['gpu-1'],
    allocatedCores: 100,
    allocatedMem: 1024,
    createTime: '2026-08-31T00:00:00Z'
  })
  const createGate = () => {
    let release
    const promise = new Promise((resolve) => {
      release = resolve
    })
    return { promise, release }
  }

  await page.route('**/api/vgpu/v1/containers', async(route) => {
    const handler = responses.shift()
    assert.ok(handler, 'Workload list issued an unexpected request')
    receivedRequests += 1
    await handler(route)
    completedRequests += 1
  })

  const initialGate = createGate()
  enqueue(async(route) => {
    await initialGate.promise
    await fulfill(route, { invalid: true })
  })

  try {
    await page.goto(
      `${target}${basePath}admin/vgpu/task/admin`,
      { waitUntil: 'domcontentloaded' }
    )
    await page.locator('[data-testid="stateful-table-skeleton"]').waitFor()
    assert.equal(
      await page.locator('.stateful-table').getAttribute('aria-busy'),
      'true'
    )

    initialGate.release()
    await page.locator('[data-testid="stateful-table-error"]').waitFor()
    await page.getByText('The server returned an invalid list response. Please try again.')
      .waitFor()

    enqueue((route) => fulfill(route, [], 503))
    await page.locator('[data-testid="stateful-table-retry"]').click()
    await page.getByText('Failed to load this resource. Please try again.').waitFor()

    enqueue((route) => fulfill(route, []))
    await page.locator('[data-testid="stateful-table-retry"]').click()
    await page.locator('[data-testid="stateful-table-empty"]').waitFor()
    assert.equal(
      await page.locator('[data-testid="stateful-table-error"]').count(),
      0
    )

    const refreshButton = page.locator('.table-toolbar-right .t-button').nth(1)
    enqueue((route) => fulfill(route, [workload('stable-worker')]))
    await refreshButton.click()
    await page.locator('.workload-table .ellipsis-text')
      .filter({ hasText: 'stable-worker' })
      .waitFor()

    const failedRefreshGate = createGate()
    enqueue(async(route) => {
      await failedRefreshGate.promise
      await fulfill(route, [], 503)
    })
    await refreshButton.click()
    await page.locator('[data-testid="stateful-table-refreshing"]').waitFor()
    await page.locator('.workload-table .ellipsis-text')
      .filter({ hasText: 'stable-worker' })
      .waitFor()

    failedRefreshGate.release()
    await page.locator('[data-testid="stateful-table-refresh-error"]').waitFor()
    await page.locator('.workload-table .ellipsis-text')
      .filter({ hasText: 'stable-worker' })
      .waitFor()

    enqueue((route) => fulfill(route, [workload('fixed-worker')]))
    await page.locator('[data-testid="stateful-table-refresh-error"] .t-button').click()
    await page.locator('.workload-table .ellipsis-text')
      .filter({ hasText: 'fixed-worker' })
      .waitFor()

    const slowRefreshGate = createGate()
    enqueue(async(route) => {
      await slowRefreshGate.promise
      await fulfill(route, [workload('stale-worker')])
    })
    const requestsBeforeRace = receivedRequests
    await refreshButton.click()
    await waitUntil(
      () => receivedRequests === requestsBeforeRace + 1,
      'The slow list refresh did not start'
    )

    enqueue((route) => fulfill(route, [workload('newest-worker')]))
    await refreshButton.click()
    await page.locator('.workload-table .ellipsis-text')
      .filter({ hasText: 'newest-worker' })
      .waitFor()

    const completedBeforeSlowRelease = completedRequests
    slowRefreshGate.release()
    await waitUntil(
      () => completedRequests === completedBeforeSlowRelease + 1,
      'The slow list refresh did not settle'
    )
    assert.equal(
      await page.locator('.workload-table .ellipsis-text')
        .filter({ hasText: 'newest-worker' })
        .count(),
      1
    )
    assert.equal(
      await page.locator('.workload-table .ellipsis-text')
        .filter({ hasText: 'stale-worker' })
        .count(),
      0
    )
    assert.equal(responses.length, 0)
  } finally {
    initialGate.release()
    await page.close()
  }
}, { timeout: 60_000 })

test('detail pages expose truthful asynchronous resource states', async(t) => {
  const target = await startWebEntry({ frameAncestors: undefined })

  await t.test('delayed card detail shows a busy skeleton before monitoring starts', async() => {
    const requestKey = 'gpu:gpu-delayed'
    const gate = createResponseGate(requestKey)
    const page = await browser.newPage()
    const monitorRequests = trackMonitorRequests(page)
    try {
      await page.goto(
        `${target}${basePath}admin/vgpu/card/admin/gpu-delayed`,
        { waitUntil: 'domcontentloaded' }
      )
      await page.locator('[data-testid="detail-page-skeleton"]').waitFor()
      await waitUntil(
        () => (backendRequestCounts.get(requestKey) ?? 0) > 0,
        'Delayed card detail request did not reach the backend'
      )
      assert.equal(
        await page.locator('.detail-page-state').getAttribute('aria-busy'),
        'true'
      )
      assert.equal(
        monitorRequests.length,
        0,
        'Monitoring started before the card identity resolved'
      )

      gate.release()
      await page.locator('[data-testid="detail-page-skeleton"]').waitFor({
        state: 'detached'
      })
      assert.equal(
        await page.locator('.detail-page-state').getAttribute('aria-busy'),
        'false'
      )
      await page.locator('.layout-title').filter({ hasText: 'gpu-delayed' }).waitFor()
      await waitUntil(
        () => monitorRequests.length > 0,
        'Monitoring did not start after the card detail resolved'
      )
    } finally {
      gate.release()
      responseGates.delete(requestKey)
      await page.close()
    }
  })

  await t.test('zero-value card and task replies are missing, not malformed', async(t) => {
    await t.test('card', () => assertMissingDetail(
      target,
      `${basePath}admin/vgpu/card/admin/gpu-missing`
    ))
    await t.test('task', () => assertMissingDetail(
      target,
      `${basePath}admin/vgpu/task/admin/detail?name=missing-worker&podUid=pod-missing`
    ))
  })

  await t.test('a failed node detail can be retried', async() => {
    const requestKey = 'node:node-retry'
    const initialAttempts = backendRequestCounts.get(requestKey) ?? 0
    const page = await browser.newPage()
    try {
      await page.goto(
        `${target}${basePath}admin/vgpu/node/admin/node-retry?nodeName=node-retry`,
        { waitUntil: 'domcontentloaded' }
      )
      await page.locator('[data-testid="detail-page-error"]').waitFor()
      assert.equal(
        (backendRequestCounts.get(requestKey) ?? 0) - initialAttempts,
        1
      )

      await page.locator('[data-testid="detail-page-retry"]').click()
      await page.locator(
        '.detail-page-state[data-detail-state="ready"]'
      ).waitFor()
      await page.locator('.layout-title').filter({ hasText: 'node-retry' }).waitFor()
      assert.equal(
        (backendRequestCounts.get(requestKey) ?? 0) - initialAttempts,
        2
      )
    } finally {
      await page.close()
    }
  })

  await t.test('route changes never expose metrics from the previous card', async() => {
    const requestKey = 'monitor:gpu-metric-new'
    const gate = createResponseGate(requestKey)
    const page = await browser.newPage()
    try {
      await page.goto(
        `${target}${basePath}admin/vgpu/card/admin/gpu-metric-old`,
        { waitUntil: 'domcontentloaded' }
      )
      await page.locator(
        '.detail-page-state[data-detail-state="ready"]'
      ).waitFor()
      await page.waitForFunction(() =>
        [...document.querySelectorAll('.resource-card-footer-percent')]
          .some((element) => element.textContent?.trim() !== '--')
      )

      await page.evaluate(async() => {
        const app = document.querySelector('#app')?.__vue_app__
        await app?.config.globalProperties.$router.push(
          '/admin/vgpu/card/admin/gpu-metric-new'
        )
      })
      await page.locator('.layout-title').filter({ hasText: 'gpu-metric-new' }).waitFor()
      await page.locator(
        '.detail-page-state[data-detail-state="ready"]'
      ).waitFor()
      await waitUntil(
        () => (backendRequestCounts.get(requestKey) ?? 0) > 0,
        'New card monitoring request did not reach the backend'
      )

      assert.deepEqual(
        await page.locator('.resource-card-footer-percent').allTextContents(),
        ['--', '--', '--', '--']
      )

      gate.release()
      await page.waitForFunction(() =>
        [...document.querySelectorAll('.resource-card-footer-percent')]
          .some((element) => element.textContent?.trim() !== '--')
      )
    } finally {
      gate.release()
      responseGates.delete(requestKey)
      await page.close()
    }
  })
}, { timeout: 90_000 })

test('ECharts runtime renders, updates and handles interaction in Chromium', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  await assertChartRuntime(target)
}, { timeout: 60_000 })
