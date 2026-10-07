import assert from 'node:assert/strict'
import { mkdir } from 'node:fs/promises'
import http from 'node:http'
import { join } from 'node:path'
import { after, before, test } from 'node:test'
import { setTimeout as delay } from 'node:timers/promises'

import { chromium } from 'playwright'

import { expectedIconIds } from '../../packages/web/test/expected-icon-catalog.mjs'
import { buildGroupedResourceTopQueries, buildTaskAllocationTopQueries, buildTaskCountQueries } from '../../packages/web/projects/vgpu/metrics/query-contract.mjs'
import { launchWebEntry } from './launch-web-entry.mjs'

const host = '127.0.0.1'
const basePath = '/gpu-ui/'
const deepRoute = `${basePath}overview`
const longImageReference =
  'docker.io/pytorch/pytorch:2.5.1-cuda11.8-cudnn9-runtime@sha256:7aac344854fbc920da85f9abccb8e397a5bf99445553df9f4dfbde18009f4cd3'
const detailPodName = 'test-sh-65874fcfc4-ppdcc'
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

const workloadListPattern = /\/api\/vgpu\/v1\/(?:containers|workloads)$/

function fulfillWorkloadFixture(route, payload, status = 200) {
  if (new URL(route.request().url()).pathname.endsWith('/v1/workloads') && Array.isArray(payload.items)) {
    const query = route.request().postDataJSON() || {}
    const size = Math.max(1, Math.min(100, Number(query.pageSize || 10)))
    const start = (Math.max(1, Number(query.page || 1)) - 1) * size
    payload = { ...payload, total: payload.items.length, items: payload.items.slice(start, start + size) }
  }
  return route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(payload) })
}

function pendingWorkloadFixture(pod) {
  const request = pod.requests[0]
  return {
    name: request.container, appName: pod.name, podUid: pod.uid, namespace: pod.namespace,
    nodeName: '', status: 'pending', pending: true, containerKind: request.containerKind,
    createTime: pod.createdAt, scheduling: pod, request,
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

  const gaugeHelp = page.locator('.gauge-card__help').first()
  await gaugeHelp.hover()
  const gaugeTooltip = page.locator('.t-tooltip .t-popup__content').last()
  await gaugeTooltip.waitFor({ state: 'visible' })
  const gaugeTooltipBox = await gaugeTooltip.boundingBox()
  assert.ok(gaugeTooltipBox.width <= 320)
  assert.equal(
    await gaugeTooltip.evaluate((element) => getComputedStyle(element).maxWidth),
    '320px'
  )
  await page.mouse.move(0, 0)

  const distributionHelp = page.locator('.workload-distribution-tip-icon')
  await distributionHelp.scrollIntoViewIfNeeded()
  await distributionHelp.hover()
  const distributionTooltip = page.locator('.t-tooltip .t-popup__content').last()
  await distributionTooltip.waitFor({ state: 'visible' })
  const distributionTooltipBox = await distributionTooltip.boundingBox()
  assert.ok(distributionTooltipBox.width <= 320)
  assert.equal(
    await distributionTooltip.evaluate((element) => getComputedStyle(element).maxWidth),
    '320px'
  )
  await page.mouse.move(0, 0)

  const initialRangeRequests = rangeRequests
  await selectTrendRange(page, '3h')
  await waitUntil(
    () => rangeRequests > initialRangeRequests,
    'Changing the time range did not update the chart data'
  )
  assert.ok(
    await page.locator('.home-bottom-row .echarts canvas').count() >= 2,
    'Trend charts disappeared after an option update'
  )

  await assertTrendRefreshState(page)

  const pie = page.locator('.card-type-chart canvas').first()
  const pieBox = await pie.boundingBox()
  assert.ok(pieBox?.width > 0, 'Overview card-type chart has no width')
  assert.ok(pieBox?.height > 0, 'Overview card-type chart has no height')
  await pie.click({
    position: { x: pieBox.width * 0.75, y: pieBox.height * 0.5 }
  })
  await page.waitForURL((url) =>
    url.pathname.endsWith('/accelerators') &&
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
  const legendWeight = () => previewLegendItem.evaluate(
    (element) => getComputedStyle(element).fontWeight
  )
  assert.equal(await legendWeight(), '700', 'The active card type is not bold')
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
    async() => (await legendWeight()) === '400',
    'Preview chart click did not clear the active card-type filter'
  )

  // Whole-pixel scrollWidth and clientWidth cannot see a name cut by a fraction of a pixel.
  const legendName = page.locator('.nodeCard-legend .legend-name').first()
  const legendText = legendName.locator('.ellipsis-text')
  const fitLegendName = (cut) => legendName.evaluate((element, cut) => {
    const range = document.createRange()
    range.selectNodeContents(element)
    element.style.flex = 'none'
    element.style.width = `${range.getBoundingClientRect().width - cut}px`
  }, cut)
  await fitLegendName(0)
  await waitUntil(
    async() => (await legendText.getAttribute('tabindex')) === null,
    'A legend name that fits exactly still offers a tooltip'
  )
  await fitLegendName(1 / 64)
  await waitUntil(
    async() => (await legendText.getAttribute('tabindex')) === '0',
    'A legend name cut by a fraction of a pixel cannot be read in full'
  )
  const fullName = (await legendText.textContent()).trim()
  await legendText.focus()
  const legendTooltip = page.locator('[role="tooltip"].vgpu-long-text-tooltip')
    .filter({ hasText: fullName })
    .last()
  await legendTooltip.waitFor({ state: 'visible' })
  await legendText.press('Escape')
  await legendTooltip.waitFor({ state: 'hidden' })
  await legendName.evaluate((element) => element.removeAttribute('style'))

  assert.deepEqual(runtimeErrors, [])
  await page.close()
}

// A refreshing trend keeps its chart, blocks hover and zoom at once, and shows its
// indicator only once the refresh has lasted past a short delay.
async function assertTrendRefreshState(page) {
  const trend = page.locator('.home-bottom-row .metric-chart').first()
  const canvas = trend.locator('canvas').first()
  const refreshState = () => trend.evaluate((element) => {
    const overlay = element.querySelector('.metric-chart__updating')
    const tooltip = [...element.querySelectorAll('div')]
      .find((node) => node.style.zIndex === '9999999')
    const tooltipStyle = tooltip && getComputedStyle(tooltip)
    return {
      blocking: Boolean(overlay),
      visible: Boolean(overlay?.classList.contains('is-visible')),
      busy: element.getAttribute('aria-busy') === 'true',
      canvas: Boolean(element.querySelector('canvas')),
      refreshError: Boolean(element.querySelector('.metric-chart__refresh--error')),
      bodyHeight: element.querySelector('.metric-chart__body').getBoundingClientRect().height,
      tooltip: Boolean(tooltipStyle && tooltipStyle.display !== 'none' &&
        tooltipStyle.visibility !== 'hidden' && Number(tooltipStyle.opacity) > 0.5)
    }
  })
  const waitForIdle = () => waitUntil(
    async() => !(await refreshState()).busy,
    'The trend chart did not finish refreshing'
  )
  const hoverPlot = async() => {
    const box = await canvas.boundingBox()
    await page.mouse.move(box.x + box.width * 0.5, box.y + box.height * 0.4, { steps: 3 })
  }
  // Clicking without moving the pointer keeps a tooltip up unless the chart hides it.
  const selectRange = (index) => page.evaluate((position) => {
    document.querySelectorAll('.home-bottom-trend-filter .segmented-control__option')[position].click()
  }, index)

  let mode = 'pass'
  const held = []
  await page.route('**/v1/monitor/query/range-vector', async(route) => {
    if (mode === 'pass') return route.continue()
    await new Promise((release) => held.push(release))
    if (mode === 'fail') return route.fulfill({ status: 500, body: 'injected failure' })
    return route.continue()
  })
  const releaseHeld = () => held.splice(0).forEach((release) => release())

  try {
    await trend.scrollIntoViewIfNeeded()
    await waitForIdle()
    const idleHeight = (await refreshState()).bodyHeight

    // A refresh that finishes inside the delay never shows the indicator.
    await selectRange(2)
    for (let elapsed = 0; elapsed < 400; elapsed += 25) {
      assert.equal((await refreshState()).visible, false, 'A quick refresh flashed the indicator')
      await page.waitForTimeout(25)
    }
    await waitForIdle()

    // The next refresh starts its own delay, and an open tooltip closes at once.
    await hoverPlot()
    await waitUntil(async() => (await refreshState()).tooltip, 'The trend tooltip did not open')
    mode = 'hold'
    const started = Date.now()
    await selectRange(3)
    await waitUntil(async() => (await refreshState()).blocking, 'A refresh did not block the chart')
    await waitUntil(async() => !(await refreshState()).tooltip, 'An open tooltip stayed up during a refresh')
    assert.equal((await refreshState()).visible, false, 'The indicator showed before its delay')
    await hoverPlot()
    await waitUntil(async() => (await refreshState()).visible, 'A slow refresh never showed its indicator')
    assert.ok(Date.now() - started >= 240, 'The indicator did not wait for its own delay')
    const slow = await refreshState()
    assert.equal(slow.canvas, true, 'The previous chart disappeared during a refresh')
    assert.equal(slow.tooltip, false, 'The chart answered hover during a refresh')
    assert.equal(slow.bodyHeight, idleHeight, 'The refresh indicator changed the chart height')
    mode = 'pass'
    releaseHeld()
    await waitForIdle()
    assert.equal((await refreshState()).blocking, false)
    await hoverPlot()
    await waitUntil(async() => (await refreshState()).tooltip, 'Hover did not return after a refresh')

    // A failed refresh keeps the previous chart and says so.
    await page.mouse.move(0, 0)
    mode = 'fail'
    await selectRange(1)
    await waitUntil(async() => (await refreshState()).visible, 'A failing refresh never showed its indicator')
    releaseHeld()
    await waitUntil(async() => (await refreshState()).refreshError, 'A failed refresh was not reported')
    const failed = await refreshState()
    assert.equal(failed.blocking, false)
    assert.equal(failed.canvas, true, 'A failed refresh removed the previous chart')
    assert.equal(failed.bodyHeight, idleHeight)
  } finally {
    // A failed assertion must not leave requests held for the rest of the journey.
    mode = 'pass'
    releaseHeld()
    await page.unroute('**/v1/monitor/query/range-vector')
  }
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

function trackTrendRequests(page) {
  const requests = []
  page.on('request', (request) => {
    if (request.url().endsWith('/v1/monitor/query/range-vector')) requests.push(request.postDataJSON())
  })
  return requests
}

const trendPages = [
  ['overview', 5],
  ['nodes/node-1?nodeName=node-1', 5],
  ['accelerators/gpu-1', 7],
  ['workloads/pod-1/containers/worker', 2]
]

function trendRangeButton(page, value) {
  const index = ['1h', '3h', '6h', '24h', '168h', '720h', 'custom'].indexOf(value)
  assert.ok(index >= 0, `Unknown trend preset: ${value}`)
  return page.locator('.trend-time-filter-presets .segmented-control__option').nth(index)
}

async function selectTrendRange(page, value) {
  const filter = page.locator('.trend-time-filter')
  const button = trendRangeButton(page, value)
  if (await button.isVisible()) return button.click()
  const label = (await button.textContent()).trim()
  await filter.locator('.trend-time-filter-select').click()
  await page.locator('.t-select-option:visible').getByText(label, { exact: true }).click()
}

async function captureTrendScreenshot(page, name) {
  const directory = process.env.WEB_ENTRY_SCREENSHOT_DIR
  if (!directory) return
  await mkdir(directory, { recursive: true })
  await page.screenshot({ path: join(directory, `${name}.png`) })
}

async function stablePopupBounds(popup) {
  let previous
  return waitUntil(async() => {
    const bounds = await popup.boundingBox()
    const snapshot = JSON.stringify(bounds)
    const animating = await popup.evaluate((element) => element.getAnimations({ subtree: true }).some((animation) => animation.playState === 'running'))
    const settled = bounds && !animating && snapshot === previous
    previous = snapshot
    return settled ? bounds : false
  }, 'Popup placement did not settle')
}

async function customTimestampFieldsFit(filter) {
  return filter.locator('.trend-time-filter-custom input').evaluateAll((elements) => elements.length === 2 && elements.every((element) => {
    const width = element.getBoundingClientRect().width
    const context = document.createElement('canvas').getContext('2d')
    const style = getComputedStyle(element)
    context.font = style.font
    return context.measureText(element.value).width <= width - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight)
  }))
}

async function assertRefreshAnimation(page, button, originalIcon) {
  assert.ok(await originalIcon.evaluate((element) => element.isConnected), 'Refreshing must retain the same refresh SVG')
  const icon = button.locator('.refresh-button__icon')
  const animation = () => icon.evaluate((element) => {
    const style = getComputedStyle(element)
    return [style.animationName, style.animationDuration, style.animationTimingFunction, style.animationIterationCount]
  })
  const [name, ...timing] = await animation()
  assert.match(name, /^refresh-button-spin/)
  assert.deepEqual(timing, ['0.9s', 'linear', 'infinite'])
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await waitUntil(async() => (await animation())[0] === 'none', 'Reduced motion must stop the refresh rotation')
  assert.equal(await button.getAttribute('aria-busy'), 'true', 'Reduced motion must retain the busy indication')
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  await waitUntil(async() => (await animation())[0] === name, 'Normal motion must restore the shared rotation')
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
        isSchedulable: uid !== 'node-cordoned',
        isReady: uid !== 'node-readability',
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
            appName: name === 'long-image-worker' ? detailPodName : 'job-1',
            nodeName: 'node-1',
            allocatedDevices: 1,
            allocatedCores: 100,
            allocatedMem: 1024,
            type: 'NVIDIA',
            podUid,
            nodeUid: 'node-1',
            namespace: 'default',
            deviceIds: ['gpu-1'],
            images: name === 'multi-image-worker'
              ? [
                  'example.invalid/worker:latest',
                  'example.invalid/sidecar:latest'
                ]
              : [name === 'long-image-worker'
                  ? longImageReference
                  : 'example.invalid/worker:latest']
          }
    } else if (pathname === '/v1/nodes') {
      payload = {
        code: 0,
        list: [
          {
            name: 'node-1',
            uid: 'node-1',
            ip: '192.0.2.10',
            isExternal: false,
            isReady: true,
            isSchedulable: true
          },
          {
            name: 'node-readability',
            uid: 'node-readability',
            ip: '192.0.2.11',
            isExternal: false,
            isReady: false,
            isSchedulable: true
          },
          {
            name: 'node-cordoned',
            uid: 'node-cordoned',
            ip: '192.0.2.12',
            isExternal: false,
            isReady: true,
            isSchedulable: false
          }
        ],
        total: 3
      }
    } else if (pathname === '/v1/gpus') {
      payload = {
        code: 0,
        list: [{ uuid: 'gpu-1', type: 'NVIDIA', node: 'node-1', health: true }],
        total: 1
      }
    } else if (pathname === '/v1/containers' || pathname === '/v1/workloads') {
      payload = { code: 0, items: [{ name: 'worker', podName: 'job-1', appName: 'job-1', podUid: 'pod-1', namespace: 'default', nodeName: 'node-1' }], total: 1 }
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
      `${target}${basePath}workloads`,
      { waitUntil: 'domcontentloaded' }
    )
    const englishButton = page.getByRole('button', { name: 'English', exact: true })
    await englishButton.waitFor()
    assert.equal(await englishButton.getAttribute('aria-pressed'), 'true')
    await page.locator('.workload-table .vgpu-table-name-text-wrap .ellipsis-text')
      .filter({ hasText: 'worker' })
      .waitFor()

    const requestsCard = page.locator('.task-top-box .home-block').nth(1)
    await requestsCard.locator('.title')
      .filter({ hasText: 'Workload Allocation Top5' })
      .waitFor()
    await requestsCard.getByRole('button', { name: 'vGPU', exact: true }).click()
    const value = requestsCard.locator('.tab-top-value').first()
    await value.waitFor()
    assert.equal((await value.textContent()).trim(), '0.4 slots')

    await page.getByRole('button', { name: 'Collapse sidebar', exact: true }).click()
    await page.getByRole('button', { name: 'Switch to 中文', exact: true }).click()
    await page.locator('html[lang="zh-CN"]').waitFor()
    assert.equal(await page.locator('.sidebar.is-collapsed').count(), 1)
    assert.equal(await page.locator('.lang-dropdown-popper:visible').count(), 0)
    await page.getByRole('button', { name: '切换到 English', exact: true }).click()
    await page.locator('html[lang="en"]').waitFor()
    await page.getByRole('button', { name: 'Expand sidebar', exact: true }).click()
    assert.equal(await englishButton.getAttribute('aria-pressed'), 'true')
    assert.equal(page.url(), `${target}${basePath}workloads`)

    await page.goto(`${target}${basePath}401`, { waitUntil: 'domcontentloaded' })
    await page.getByRole('heading', { name: 'Page not found' }).waitFor()
    assert.equal((await page.locator('body').textContent()).includes('页面未找到'), false)
  } finally {
    await page.close()
  }
}, { timeout: 60_000 })

test('workload and detail views keep dense identity content readable', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({
    locale: 'en-US',
    viewport: { width: 1280, height: 900 }
  })
  const podName =
    'distributed-training-pod-with-a-name-long-enough-to-require-ellipsis'
  const workloads = [
    {
      name: 'worker-left',
      appName: podName,
      podUid: 'pod-readable',
      namespace: 'research-space',
      nodeName: 'node-1',
      status: 'success',
      deviceIds: ['gpu-1'],
      allocatedCores: 30,
      allocatedMem: 2048,
      createTime: '2026-08-31T00:00:00Z'
    },
    {
      name: 'worker-right',
      appName: podName,
      podUid: 'pod-readable',
      namespace: 'research-space',
      nodeName: 'node-1',
      status: 'success',
      deviceIds: ['gpu-1'],
      allocatedCores: 30,
      allocatedMem: 2048,
      createTime: '2026-08-31T00:00:00Z'
    },
    {
      name: 'torch-two-gpu',
      appName: 'torch-two-gpu',
      podUid: 'pod-same-name',
      namespace: 'research-space',
      nodeName: 'node-1',
      status: 'success',
      deviceIds: ['gpu-1'],
      allocatedCores: 50,
      allocatedMem: 4096,
      createTime: '2026-08-31T00:00:00Z'
    }
  ]

  await page.route(workloadListPattern, (route) => fulfillWorkloadFixture(route, { code: 0, items: workloads, total: workloads.length }))

  const assertIconGeometry = async(selector, outerSize = [40, 40]) => {
    const boxes = await page.locator(selector).evaluateAll((elements) =>
      elements.map((element) => {
        const icon = element.querySelector('svg')
        const outer = element.getBoundingClientRect()
        const inner = icon?.getBoundingClientRect()
        return {
          outer: [outer.width, outer.height],
          inner: [inner?.width ?? 0, inner?.height ?? 0]
        }
      })
    )
    assert.ok(boxes.length > 0, `No icons matched ${selector}`)
    for (const box of boxes) {
      assert.deepEqual(box.outer, outerSize)
      assert.deepEqual(box.inner, [20, 20])
    }
  }

  const assertNoHorizontalOverflow = async() => {
    const dimensions = await page.evaluate(() => ({
      document: document.documentElement.scrollWidth,
      viewport: window.innerWidth
    }))
    assert.ok(
      dimensions.document <= dimensions.viewport,
      `Page overflows horizontally: ${JSON.stringify(dimensions)}`
    )
  }

  try {
    await page.goto(
      `${target}${basePath}workloads`,
      { waitUntil: 'domcontentloaded' }
    )
    const identities = page.locator('.workload-table .workload-identity')
    await identities.first().waitFor()
    assert.equal(await identities.count(), 3)
    assert.equal(
      (await identities.first().locator('.workload-pod-name').textContent()).trim(),
      'distribu…lipsis'
    )
    assert.equal(
      (await identities.first().locator('.workload-container-name').textContent()).trim(),
      'worker-left'
    )
    assert.equal(
      (await identities.nth(1).locator('.workload-container-name').textContent()).trim(),
      'worker-right'
    )
    const sameNameLink = page.locator('.workload-identity-link[aria-label="torch-two-gpu"]')
    assert.equal(await sameNameLink.count(), 1)
    assert.equal(await sameNameLink.locator('.workload-pod-name').count(), 0)
    assert.equal(
      (await sameNameLink.locator('.workload-container-name').textContent()).trim(),
      'torch-two-gpu'
    )
    assert.equal(
      (await identities.first().locator('.workload-namespace-label').textContent()).trim(),
      'Namespace:'
    )
    assert.equal(
      (await identities.first().locator('.task-namespace-text').textContent()).trim(),
      'research-space'
    )
    const workloadLink = identities.first().locator('.workload-identity-link')
    assert.equal(await workloadLink.count(), 1)
    assert.equal(await identities.first().locator('a').count(), 1)
    assert.equal(await workloadLink.getAttribute('aria-label'), `${podName} / worker-left`)
    assert.equal(
      await identities.first().locator('.workload-container-name .el-tooltip__trigger').count(),
      0
    )
    assert.equal(
      await sameNameLink.locator('.workload-container-name .el-tooltip__trigger').count(),
      0
    )
    await workloadLink.hover()
    const workloadLinkDecoration = await workloadLink.evaluate((element) => {
      const textRect = (target) => {
        const range = document.createRange()
        range.selectNodeContents(target)
        return range.getBoundingClientRect()
      }
      const label = element.querySelector('.workload-identity-label').getBoundingClientRect()
      const pod = element.querySelector('.workload-pod-name').getBoundingClientRect()
      const podText = element.querySelector('.workload-pod-name .ellipsis-text')
      const separator = element.querySelector('.workload-identity-separator')
      const container = element.querySelector('.workload-container-name').getBoundingClientRect()
      const decoration = getComputedStyle(element.querySelector('.workload-identity-label'), '::after')
      const podTextRect = textRect(podText)
      const separatorTextRect = textRect(separator)
      const containerTextRect = textRect(element.querySelector('.workload-container-name .ellipsis-text'))
      return {
        decorationBottom: decoration.bottom,
        decorationHeight: decoration.height,
        decorationOpacity: decoration.opacity,
        labelLeft: label.left,
        labelRight: label.right,
        podLeft: pod.left,
        containerRight: container.right,
        podHasSecondaryOverflow: podText.scrollWidth > podText.clientWidth,
        podTextBottom: podTextRect.bottom,
        separatorTextBottom: separatorTextRect.bottom,
        containerTextBottom: containerTextRect.bottom,
      }
    })
    assert.equal(workloadLinkDecoration.decorationBottom, '0px')
    assert.equal(workloadLinkDecoration.decorationHeight, '1px')
    assert.equal(workloadLinkDecoration.decorationOpacity, '1')
    assert.ok(Math.abs(workloadLinkDecoration.labelLeft - workloadLinkDecoration.podLeft) <= 0.5)
    assert.ok(Math.abs(workloadLinkDecoration.labelRight - workloadLinkDecoration.containerRight) <= 0.5)
    assert.equal(workloadLinkDecoration.podHasSecondaryOverflow, false)
    assert.ok(Math.abs(workloadLinkDecoration.podTextBottom - workloadLinkDecoration.separatorTextBottom) <= 0.5)
    assert.ok(Math.abs(workloadLinkDecoration.separatorTextBottom - workloadLinkDecoration.containerTextBottom) <= 0.5)
    const namespaceAlignment = await identities.first().locator('.workload-namespace-line')
      .evaluate((element) => {
        const textRect = (target) => {
          const range = document.createRange()
          range.selectNodeContents(target)
          return range.getBoundingClientRect()
        }
        const label = textRect(element.querySelector('.workload-namespace-label'))
        const value = textRect(element.querySelector('.task-namespace-text .ellipsis-text'))
        return {
          topDelta: Math.abs(label.top - value.top),
          bottomDelta: Math.abs(label.bottom - value.bottom),
        }
      })
    assert.ok(namespaceAlignment.topDelta <= 0.5)
    assert.ok(namespaceAlignment.bottomDelta <= 0.5)
    const sameNameLayout = await sameNameLink.evaluate((element) => {
      const label = element.querySelector('.workload-identity-label').getBoundingClientRect()
      const container = element.querySelector('.workload-container-name').getBoundingClientRect()
      return {
        leftDelta: Math.abs(label.left - container.left),
        rightDelta: Math.abs(label.right - container.right),
      }
    })
    assert.ok(sameNameLayout.leftDelta <= 0.5)
    assert.ok(sameNameLayout.rightDelta <= 0.5)
    const firstRowLayout = await identities.first().locator('xpath=ancestor::tr')
      .evaluate((element) => ({ rowHeight: element.getBoundingClientRect().height }))
    assert.ok(
      firstRowLayout.rowHeight <= 76,
      `Workload row is too tall: ${JSON.stringify(firstRowLayout)}`
    )
    const primaryLayout = await identities.first().locator('.workload-identity-primary')
      .evaluate((element) => {
        const pod = element.querySelector('.workload-pod-name').getBoundingClientRect()
        const separator = element.querySelector('.workload-identity-separator').getBoundingClientRect()
        const container = element.querySelector('.workload-container-name').getBoundingClientRect()
        return {
          podToSeparator: separator.x - (pod.x + pod.width),
          separatorToContainer: container.x - (separator.x + separator.width),
        }
      })
    assert.ok(primaryLayout.podToSeparator >= 0 && primaryLayout.podToSeparator <= 7)
    assert.ok(primaryLayout.separatorToContainer >= 0 && primaryLayout.separatorToContainer <= 7)
    const containerLayout = await identities.first().locator('.workload-container-name .ellipsis-text')
      .evaluate((element) => ({
        clientWidth: element.clientWidth,
        scrollWidth: element.scrollWidth,
        flexShrink: getComputedStyle(element.closest('.workload-container-name')).flexShrink,
      }))
    assert.equal(containerLayout.clientWidth, containerLayout.scrollWidth)
    assert.equal(containerLayout.flexShrink, '0')
    await page.getByRole('columnheader', { name: 'Accelerator Configuration' }).waitFor()
    await assertIconGeometry('.task-name-icon-card')

    const podText = identities.first().locator('.ellipsis-text').first()
    await podText.hover()
    const podTooltip = page.locator('[role="tooltip"]').filter({ hasText: podName }).last()
    await podTooltip.waitFor({ state: 'visible' })
    assert.equal((await podTooltip.textContent()).trim(), podName)

    await page.goto(
      `${target}${basePath}workloads/pod-long-image/containers/long-image-worker`,
      { waitUntil: 'domcontentloaded' }
    )
    const detailIdentityRows = page.locator('.basic-info-summary .summary-item').filter({
      has: page.locator('.summary-identity-value')
    })
    await detailIdentityRows.first().waitFor()
    assert.equal(await detailIdentityRows.count(), 2)
    assert.equal(
      (await detailIdentityRows.nth(0).locator('.summary-identity-value').textContent()).trim(),
      detailPodName
    )
    assert.equal(
      (await detailIdentityRows.nth(1).locator('.summary-identity-value').textContent()).trim(),
      'long-image-worker'
    )
    assert.equal(await detailIdentityRows.locator('.ellipsis-text').count(), 0)
    const detailIdentityAlignment = await detailIdentityRows.evaluateAll((rows) =>
      rows.map((row) => {
        const label = row.querySelector('.summary-item-label').getBoundingClientRect()
        const value = row.querySelector('.summary-identity-value').getBoundingClientRect()
        return {
          centerDelta: Math.abs((label.top + label.bottom) / 2 - (value.top + value.bottom) / 2),
          overflowWrap: getComputedStyle(row.querySelector('.summary-identity-value')).overflowWrap,
        }
      })
    )
    for (const layout of detailIdentityAlignment) {
      assert.ok(layout.centerDelta <= 0.5, JSON.stringify(layout))
      assert.equal(layout.overflowWrap, 'anywhere')
    }
    const imageReference = page.locator('.summary-item-image .ellipsis-text')
    await imageReference.waitFor()
    assert.equal((await imageReference.textContent()).trim(), longImageReference)
    const imageLayout = await imageReference.evaluate((element) => {
      const style = getComputedStyle(element)
      return {
        clientWidth: element.clientWidth,
        overflow: style.overflow,
        scrollWidth: element.scrollWidth,
        textOverflow: style.textOverflow,
        whiteSpace: style.whiteSpace
      }
    })
    assert.equal(imageLayout.overflow, 'hidden')
    assert.equal(imageLayout.textOverflow, 'ellipsis')
    assert.equal(imageLayout.whiteSpace, 'nowrap')
    assert.ok(imageLayout.scrollWidth > imageLayout.clientWidth)
    await imageReference.hover()
    const imageTooltip = page.locator('[role="tooltip"].vgpu-long-text-tooltip')
      .filter({ hasText: longImageReference })
      .last()
    await imageTooltip.waitFor({ state: 'visible' })
    assert.equal((await imageTooltip.textContent()).trim(), longImageReference)
    const imageTooltipBox = await imageTooltip.boundingBox()
    assert.ok(imageTooltipBox.width <= 320)
    assert.equal(
      await imageTooltip.evaluate((element) => getComputedStyle(element).maxWidth),
      '320px'
    )
    await assertNoHorizontalOverflow()

    await page.goto(
      `${target}${basePath}workloads/pod-short-image/containers/short-image-worker`,
      { waitUntil: 'domcontentloaded' }
    )
    const shortImageReference = page.locator('.summary-item-image .ellipsis-text')
    await shortImageReference.waitFor()
    assert.equal(
      (await shortImageReference.textContent()).trim(),
      'example.invalid/worker:latest'
    )
    const shortImageLayout = await shortImageReference.evaluate((element) => ({
      clientWidth: element.clientWidth,
      cursor: getComputedStyle(element).cursor,
      scrollWidth: element.scrollWidth,
    }))
    assert.equal(shortImageLayout.clientWidth, shortImageLayout.scrollWidth)
    assert.notEqual(shortImageLayout.cursor, 'help')
    await shortImageReference.hover()
    await page.waitForTimeout(400)
    assert.equal(
      await page.locator('[role="tooltip"]:visible')
        .filter({ hasText: 'example.invalid/worker:latest' })
        .count(),
      0
    )

    await page.goto(
      `${target}${basePath}workloads/pod-multi-image/containers/multi-image-worker`,
      { waitUntil: 'domcontentloaded' }
    )
    const multiImageReference = page.locator('.summary-item-image .image-reference')
    await multiImageReference.waitFor()
    assert.equal(
      (await multiImageReference.textContent()).trim(),
      'example.invalid/worker:latest +1'
    )
    await page.mouse.move(0, 0)
    await multiImageReference.focus()
    const multiImageTooltip = page.locator('.t-tooltip .t-popup__content')
      .filter({ hasText: 'example.invalid/sidecar:latest' })
      .last()
    await multiImageTooltip.waitFor({ state: 'visible' })
    assert.equal(
      (await multiImageTooltip.textContent()).trim(),
      'example.invalid/worker:latest\nexample.invalid/sidecar:latest'
    )

    await multiImageReference.hover()
    await page.mouse.move(0, 0)
    assert.equal(await multiImageTooltip.isVisible(), true)
    await multiImageReference.press('Tab')
    await multiImageTooltip.waitFor({ state: 'hidden' })

    await multiImageReference.hover()
    await multiImageTooltip.waitFor({ state: 'visible' })
    await multiImageReference.focus()
    await multiImageReference.press('Tab')
    assert.equal(await multiImageTooltip.isVisible(), true)
    await page.mouse.move(0, 0)
    await multiImageTooltip.waitFor({ state: 'hidden' })

    await page.setViewportSize({ width: 300, height: 900 })
    await multiImageReference.focus()
    await multiImageTooltip.waitFor({ state: 'visible' })
    const narrowImageTooltipBox = await multiImageTooltip.boundingBox()
    assert.ok(narrowImageTooltipBox.width <= 268, JSON.stringify(narrowImageTooltipBox))
    await multiImageReference.press('Escape')
    await multiImageTooltip.waitFor({ state: 'hidden' })
    await page.setViewportSize({ width: 1280, height: 900 })

    await page.goto(
      `${target}${basePath}nodes`,
      { waitUntil: 'domcontentloaded' }
    )
    const nodeTable = page.locator('.node-table')
    await nodeTable.getByRole('columnheader', { name: 'Status' }).waitFor()
    assert.equal(await nodeTable.getByRole('columnheader', { name: 'Readiness' }).count(), 0)
    assert.equal(await nodeTable.getByRole('columnheader', { name: 'Scheduling' }).count(), 0)
    const readyRow = nodeTable.getByRole('row').filter({ hasText: 'node-1' })
    const notReadyRow = nodeTable.getByRole('row').filter({ hasText: '192.0.2.11' })
    assert.equal((await readyRow.textContent()).includes('Schedulable'), true)
    assert.equal(
      await readyRow.getByRole('button', { name: 'View node scheduling status details' }).count(),
      0
    )
    const listStatusHelp = notReadyRow.getByRole('button', {
      name: 'View node scheduling status details',
    })
    assert.equal(
      await listStatusHelp.count(),
      1,
      `Expected help only for the abnormal row: ${(await notReadyRow.textContent()).trim()}`
    )
    await listStatusHelp.hover()
    const listStatusTooltip = page.locator('.t-tooltip .t-popup__content')
      .filter({ hasText: 'The node is not in the Ready state' })
      .last()
    await listStatusTooltip.waitFor({ state: 'visible' })
    const listStatusTooltipBox = await listStatusTooltip.boundingBox()
    assert.ok(listStatusTooltipBox.width <= 320)
    assert.equal(
      await listStatusTooltip.evaluate((element) => getComputedStyle(element).maxWidth),
      '320px'
    )

    await page.goto(
      `${target}${basePath}nodes/node-1?nodeName=node-1`,
      { waitUntil: 'domcontentloaded' }
    )
    await page.locator('.detail-page-state[data-detail-state="ready"]').waitFor()
    assert.equal(
      (await page.locator('.layout-header-title-run-state-label').textContent()).trim(),
      'Schedulable'
    )
    const schedulableHelp = page.getByRole('button', {
      name: 'View node scheduling status details',
    })
    assert.equal(await schedulableHelp.count(), 0)

    await page.goto(
      `${target}${basePath}nodes/node-readability?nodeName=node-readability`,
      { waitUntil: 'domcontentloaded' }
    )
    await page.locator('.detail-page-state[data-detail-state="ready"]').waitFor()
    assert.equal(
      (await page.locator('.layout-header-title-run-state-label').textContent()).trim(),
      'Temporarily Unschedulable'
    )
    const notReadyHelp = page.getByRole('button', {
      name: 'View node scheduling status details',
    })
    await notReadyHelp.hover()
    await page.locator('.t-tooltip .t-popup__content')
      .filter({ hasText: 'The node is not in the Ready state' })
      .last()
      .waitFor({ state: 'visible' })
    const nodeSummaryLabels = (await page.locator('.summary-item-label').allTextContents())
      .map((value) => value.trim())
    assert.equal(nodeSummaryLabels.includes('Scheduling'), false)
    assert.equal(nodeSummaryLabels.includes('Readiness'), false)
    assert.deepEqual(
      (await page.locator('.resource-card-footer-label').allTextContents())
        .map((value) => value.trim()),
      ['Alloc Rate', 'Usage Rate', 'Alloc Rate', 'Usage Rate']
    )
    // The resource cards follow their panel's width: compact beside the sidebar at 1280 px, full size with more room.
    const resourcePanelWidth = () => page.locator('.node-workload-panel').evaluate((element) => element.clientWidth)
    const compactPanelWidth = await resourcePanelWidth()
    await assertIconGeometry('.resource-card-icon', [32, 32])
    await page.setViewportSize({ width: 1600, height: 900 })
    await waitUntil(
      async() => (await resourcePanelWidth()) > compactPanelWidth + 100,
      'The node resource panel did not widen with the viewport'
    )
    await assertIconGeometry('.resource-card-icon', [40, 40])
    await page.setViewportSize({ width: 1280, height: 900 })
    await waitUntil(
      async() => (await resourcePanelWidth()) === compactPanelWidth,
      'The node resource panel did not return to its 1280 px width'
    )
    const rateTiles = page.locator('.resource-overview-card').first()
      .locator('.resource-card-rate-wrap')
    const rateTileBoxes = await rateTiles.evaluateAll((elements) =>
      elements.map((element) => {
        const box = element.getBoundingClientRect()
        const title = element.querySelector('.resource-card-footer-title')
          .getBoundingClientRect()
        const value = element.querySelector('.resource-card-footer-value')
          .getBoundingClientRect()
        return {
          x: box.x,
          y: box.y,
          width: box.width,
          height: box.height,
          titleCenterY: title.y + title.height / 2,
          valueCenterY: value.y + value.height / 2,
        }
      })
    )
    assert.equal(rateTileBoxes.length, 2)
    assert.ok(Math.abs(rateTileBoxes[0].x - rateTileBoxes[1].x) < 1)
    assert.ok(rateTileBoxes[0].y + rateTileBoxes[0].height <= rateTileBoxes[1].y)
    for (const tile of rateTileBoxes) {
      assert.ok(Math.abs(tile.titleCenterY - tile.valueCenterY) < 2)
    }
    await assertNoHorizontalOverflow()

    await page.goto(
      `${target}${basePath}nodes/node-cordoned?nodeName=node-cordoned`,
      { waitUntil: 'domcontentloaded' }
    )
    await page.locator('.detail-page-state[data-detail-state="ready"]').waitFor()
    assert.equal(
      (await page.locator('.layout-header-title-run-state-label').textContent()).trim(),
      'Temporarily Unschedulable'
    )
    const cordonedHelp = page.getByRole('button', {
      name: 'View node scheduling status details',
    })
    await cordonedHelp.hover()
    await page.locator('.t-tooltip .t-popup__content')
      .filter({ hasText: 'The node is cordoned' })
      .last()
      .waitFor({ state: 'visible' })

    await page.goto(
      `${target}${basePath}accelerators/gpu-1`,
      { waitUntil: 'domcontentloaded' }
    )
    await page.locator('.detail-page-state[data-detail-state="ready"]').waitFor()
    assert.deepEqual(
      (await page.locator('.resource-card-footer-label').allTextContents())
        .map((value) => value.trim()),
      [
        'Allocated',
        'Used',
        'Allocated',
        'Used'
      ]
    )
    await assertIconGeometry('.resource-card-icon')
    await assertNoHorizontalOverflow()
  } finally {
    await page.close()
  }
}, { timeout: 60_000 })

test('GPU resource cards stay compact across panel widths and languages', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US', viewport: { width: 1366, height: 900 } })
  let deviceMode = 'hami-core'
  let longValues = false
  await page.route('**/v1/gpu?**', async(route) => {
    const response = await route.fetch()
    const device = await response.json()
    return route.fulfill({ json: { ...device, vendor: 'NVIDIA', mode: deviceMode } })
  })
  await page.route('**/v1/monitor/query/instant-vector', (route) => {
    const { query } = route.request().postDataJSON()
    let value = 100
    if (query.includes('hami_container_vcore_allocation_known')) value = 0
    else if (query.includes('hami_container_vmemory_allocated')) value = longValues ? 2048.75 : 512.25
    else if (query.includes('hami_vmemory_size')) value = 1024.5
    else if (query.includes('hami_memory_used')) value = 128.25
    else if (query.includes('hami_memory_size')) value = 2048.5
    else if (query.includes('hami_container_vcore_allocated')) value = 85.25
    else if (query.includes('hami_core_util')) value = 64.25
    return route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ data: [{ metric: { device_uuid: 'gpu-1' }, value }] })
    })
  })
  const inspectLayout = () => page.locator('.resource-overview-cards').evaluate((element) => {
    const rect = (node) => {
      const { x, y, width, height, right, bottom } = node.getBoundingClientRect()
      return { x, y, width, height, right, bottom }
    }
    const overview = element.closest('.resource-overview-layout')
    const gauge = overview.querySelector('.resource-slot-card')
    return {
      ...rect(element),
      width: element.clientWidth,
      scrollWidth: element.scrollWidth,
      overview: { ...rect(overview), scrollWidth: overview.scrollWidth },
      gauge: gauge ? rect(gauge) : null,
      gaugeContent: gauge ? rect(gauge.querySelector('.resource-slot-gauge')) : null,
      itemCount: element.children.length,
      cards: [...element.querySelectorAll('.resource-card')].map((card) => ({
        ...rect(card),
        rows: [...card.querySelectorAll('.resource-card-footer-item')].map((row) => {
          const title = row.querySelector('.resource-card-footer-title')
          const value = row.querySelector('.resource-card-footer-value')
          return {
            ...rect(row),
            title: rect(title),
            value: rect(value),
            titleFits: title.scrollWidth <= title.clientWidth + 1,
            valueFits: value.scrollWidth <= value.clientWidth + 1,
          }
        }),
      })),
    }
  })
  const toggleSidebar = async(name) => {
    await page.getByRole('button', { name, exact: true }).click()
    await page.locator('.page-aside').evaluate(async(element) => {
      await Promise.all(element.getAnimations().map((animation) => animation.finished))
    })
  }
  const assertLayout = async(context) => {
    const layout = await inspectLayout()
    assert.equal(layout.itemCount, 2, `${context}: gauge must not replace either resource card`)
    assert.equal(layout.cards.length, 2)
    assert.ok(layout.scrollWidth <= layout.width + 1, `${context}: resource cards overflow`)
    assert.ok(layout.overview.scrollWidth <= layout.overview.width + 1, `${context}: resource overview overflows`)
    assert.equal(Boolean(layout.gauge), deviceMode === 'hami-core', `${context}: slot gauge does not follow the sharing mode`)
    if (!layout.gauge) {
      assert.ok(Math.abs(layout.x - layout.overview.x) < 1 && Math.abs(layout.width - layout.overview.width) < 1,
        `${context}: hidden slot gauge leaves an empty column`)
    } else if (layout.overview.width >= 900) {
      assert.ok(layout.gauge.right <= layout.x, `${context}: gauge overlaps resource cards`)
      assert.ok(Math.abs(layout.gauge.y - layout.y) < 1 && Math.abs(layout.gauge.height - layout.height) < 1,
        `${context}: gauge and resource cards do not align in the wide layout`)
    } else {
      assert.ok(layout.gauge.bottom <= layout.y, `${context}: narrow layout does not place the gauge above both cards`)
      assert.ok(layout.gauge.height <= 150, `${context}: narrow gauge leaves excessive vertical space`)
      assert.ok(Math.abs(layout.gaugeContent.x + layout.gaugeContent.width / 2 -
        (layout.overview.x + layout.overview.width / 2)) < 1, `${context}: narrow gauge is not centered`)
    }
    const rows = layout.cards.flatMap((card) => card.rows)
    assert.equal(rows.length, 4)
    const narrow = layout.overview.width < 320
    if (!narrow) {
      assert.ok(Math.max(...rows.map((row) => row.height)) - Math.min(...rows.map((row) => row.height)) < 1,
        `${context}: metric rows have different heights`)
    }
    for (const row of rows) {
      if (narrow) {
        assert.ok(row.value.y >= row.title.bottom, `${context}: narrow metric row does not separate its label and reading`)
      } else {
        assert.ok(row.height <= 25 && row.value.y < row.title.bottom - 1,
          `${context}: metric row expands into separate label and value lines`)
        assert.ok(row.title.right <= row.value.x + 1, `${context}: label overlaps values`)
      }
      assert.ok(row.titleFits && row.valueFits, `${context}: metric text is clipped`)
      assert.ok(row.title.x >= row.x - 1 && row.title.right <= row.right + 1 &&
        row.value.x >= row.x - 1 && row.value.right <= row.right + 1,
      `${context}: metric content escapes its row`)
      assert.ok(row.value.bottom <= row.bottom + 1, `${context}: metric reading escapes its row vertically`)
    }
    const [compute, memory] = layout.cards
    if (page.viewportSize().width >= 1024) {
      assert.ok(Math.abs(compute.y - memory.y) < 1, `${context}: notebook viewport should keep both cards side by side`)
    } else {
      assert.ok(memory.y >= compute.bottom, `${context}: narrow viewport should stack complete cards`)
    }
    if (Math.abs(compute.y - memory.y) < 1) {
      assert.ok(Math.abs(compute.width - memory.width) < 1, `${context}: resource card widths differ`)
      assert.ok(Math.abs(compute.rows[0].y - memory.rows[0].y) < 1,
        `${context}: resource card rows are vertically misaligned`)
    }
  }
  try {
    await page.goto(`${target}${basePath}accelerators/gpu-1`, { waitUntil: 'networkidle' })
    await page.locator('.resource-card-footer-metric').filter({ hasText: '512.3 GiB' }).waitFor()
    assert.equal(
      (await page.locator('.resource-card-footer-reading').nth(2).textContent()).trim().replace(/\s+/g, ' '),
      '512.3 GiB (50%)'
    )
    await page.locator('.resource-overview-layout .workload-progress-ring').waitFor()
    for (const language of ['en', 'zh-CN']) {
      if (language === 'zh-CN') await page.getByRole('button', { name: '中文', exact: true }).click()
      await page.locator(`html[lang="${language}"]`).waitFor()
      for (const [width, height] of [[1024, 768], [1280, 800], [1366, 768], [1440, 900], [1920, 1080], [768, 900]]) {
        await page.setViewportSize({ width, height })
        await assertLayout(`${language}, ${width}px, expanded`)
        await toggleSidebar(language === 'en' ? 'Collapse sidebar' : '收起侧栏')
        await assertLayout(`${language}, ${width}px, collapsed`)
        await toggleSidebar(language === 'en' ? 'Expand sidebar' : '展开侧栏')
      }
      for (const mode of ['hami-core', 'mig']) {
        deviceMode = mode
        longValues = true
        await page.goto(`${target}${basePath}accelerators/gpu-1`, { waitUntil: 'networkidle' })
        await page.locator('.resource-card-footer-metric').filter({ hasText: '2048.8 GiB' }).waitFor()
        assert.equal(
          (await page.locator('.resource-card-footer-reading').nth(2).textContent()).trim().replace(/\s+/g, ' '),
          '2048.8 GiB (199.98%)',
          `${language}, ${mode}: overallocated memory must keep its full value and percentage`
        )
        for (const width of [1024, 600]) {
          await page.setViewportSize({ width, height: 900 })
          await assertLayout(`${language}, ${mode}, ${width}px, expanded, long values`)
        }
        await toggleSidebar(language === 'en' ? 'Collapse sidebar' : '收起侧栏')
        for (const width of [375, 390]) {
          await page.setViewportSize({ width, height: 900 })
          await assertLayout(`${language}, ${mode}, ${width}px, collapsed, long values`)
        }
        await toggleSidebar(language === 'en' ? 'Expand sidebar' : '展开侧栏')
      }
      deviceMode = 'hami-core'
      longValues = false
      await page.setViewportSize({ width: 1366, height: 900 })
      await page.goto(`${target}${basePath}accelerators/gpu-1`, { waitUntil: 'networkidle' })
      await page.locator('.resource-card-footer-metric').filter({ hasText: '512.3 GiB' }).waitFor()
    }
  } finally {
    await page.close()
  }
}, { timeout: 60_000 })

test('device holders omit duplicate shared counts while keeping allocation distribution and workload context', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US', viewport: { width: 1366, height: 900 } })
  const shared = { mode: 'hami-core', shape: 'soft', allocatedMem: 4096 }
  const cases = [
    { name: 'occupied shared device', ...shared },
    { name: 'idle shared device', ...shared, empty: true },
    { name: 'overallocated shared memory', ...shared, allocatedMem: 20_480, warning: '4 GiB over the device memory' },
    { name: 'MIG device', mode: 'mig', shape: 'mig', allocatedMem: 5120, template: '1g.5gb', migStart: 0, migSize: 1 },
    { name: 'template device', mode: 'template', shape: 'template', allocatedMem: 4096, template: 'vir01' },
  ]
  let current = cases[0]
  const workload = () => ({
    name: 'worker', appName: 'example-job', podUid: 'pod-split', namespace: 'research',
    nodeName: 'node-1', nodeUid: 'node-1', status: 'success', type: 'NVIDIA',
    deviceIds: ['gpu-1'], allocatedDevices: 1, allocatedCores: 25, allocatedMem: current.allocatedMem,
    devices: [{
      id: 'gpu-1', allocationShape: current.shape, allocatedCores: 25, allocatedCoresKnown: true,
      allocatedMem: current.allocatedMem, template: current.template,
      migStart: current.migStart, migSize: current.migSize,
    }],
  })
  await page.route('**/v1/gpu?**', (route) => route.fulfill({ json: {
    uuid: 'gpu-1', type: 'NVIDIA', vendor: 'NVIDIA', nodeName: 'node-1', nodeUid: 'node-1', health: true,
    mode: current.mode, vgpuUsed: current.empty ? 0 : 1, vgpuTotal: 10, coreTotal: 100, memoryTotal: 16_384,
    migProfiles: [{ name: '1g.5gb', placements: [{ start: 0, size: 1 }, { start: 1, size: 1 }] }],
  } }))
  await page.route(workloadListPattern, (route) => fulfillWorkloadFixture(route, {
    items: current.empty ? [] : [workload()],
  }))
  await page.route('**/v1/container?**', (route) => route.fulfill({ json: workload() }))
  try {
    for (const scenario of cases) {
      current = scenario
      await page.goto(`${target}${basePath}accelerators/gpu-1`, { waitUntil: 'networkidle' })
      const split = page.locator('.device-split-block .device-split[aria-busy="false"]')
      await split.waitFor()
      assert.equal(await split.locator('.split-meter').count(), scenario.mode === 'hami-core' && !scenario.empty ? 2 : 0, `${scenario.name}: shared allocation distribution is missing or idle bars remain`)
      assert.doesNotMatch(await split.textContent(), /Shared by/, `${scenario.name}: duplicate shared count remains`)
      assert.equal(await split.locator('.split-row').count(), scenario.empty ? 0 : 1, scenario.name)
      if (scenario.empty) {
        assert.equal((await split.locator('.device-split__empty').textContent()).trim(), 'No workload holds this device.')
        assert.equal(await split.locator('.device-split__head').count(), 0, 'Idle shared device leaves an empty summary header')
        assert.ok((await split.boundingBox()).height <= 80, 'Idle shared device leaves an oversized empty panel')
      } else {
        const holder = split.locator('.split-row')
        assert.match(await holder.textContent(), /Compute 25%/)
        assert.match(await holder.locator('.split-row__memory').textContent(), /GiB/)
        assert.match(await holder.locator('.split-row__link').getAttribute('href'), /workloads\/pod-split\/containers\/worker$/)
        if (scenario.mode === 'hami-core') {
          await holder.hover()
          assert.equal(await split.locator('.split-meter__part.is-active').count(), 2, 'Holder hover no longer identifies its memory and compute shares')
        }
      }
      if (scenario.warning) {
        assert.equal((await split.locator('.device-split__item.is-warning').textContent()).trim(), scenario.warning)
      } else if (scenario.mode === 'hami-core') {
        assert.equal(await split.locator('.device-split__head').count(), 0, `${scenario.name}: empty summary header remains`)
      }
      if (scenario.mode === 'mig') {
        assert.equal(await split.locator('.split-mig').count(), 1, 'MIG placement diagram was removed')
        assert.match(await split.locator('.device-split__summary').textContent(), /1 instance allocated/)
        assert.match(await split.locator('.device-split__summary').textContent(), /Room for 1g\.5gb ×1/)
        assert.equal((await split.locator('.split-row__slot').textContent()).trim(), 'Slice 0')
      } else if (scenario.mode === 'template') {
        assert.equal(await split.locator('.split-strip').count(), 1, 'Template partition diagram was removed')
        assert.match(await split.locator('.device-split__summary').textContent(), /4 GiB of 16 GiB allocated/)
        assert.match(await split.locator('.split-row__part').textContent(), /vir01/)
      }
    }
    current = cases[0]
    await page.goto(`${target}${basePath}workloads/pod-split/containers/worker`, { waitUntil: 'networkidle' })
    const workloadSplit = page.locator('.workload-split .device-split[aria-busy="false"]')
    await workloadSplit.waitFor()
    assert.match(await workloadSplit.locator('.device-split__summary').textContent(), /Shared by 1 of 10/)
    assert.equal(await workloadSplit.locator('.split-meter').count(), 2, 'Workload detail lost its device allocation context')
    assert.equal(await workloadSplit.locator('.split-row.is-current').count(), 1, 'Current workload is no longer highlighted')
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
      `${target}${basePath}nodes`,
      { waitUntil: 'domcontentloaded' }
    )
    await page.locator('html[lang="en"]').waitFor()

    const messageBox = page.locator('.el-message-box')
    await messageBox.waitFor()
    await messageBox.locator('.el-button--primary')
      .filter({ hasText: 'OK' })
      .click()

    await page.getByRole('button', { name: '中文', exact: true }).click()
    await page.locator('html[lang="zh-CN"]').waitFor()

    failNextNodesRequest = true
    await page.reload({ waitUntil: 'domcontentloaded' })
    await page.locator('html[lang="zh-CN"]').waitFor()
    await messageBox.waitFor()
    await messageBox.locator('.el-button--primary')
      .filter({ hasText: '确定' })
      .click()

    await page.getByRole('button', { name: 'English', exact: true }).click()
    await page.locator('html[lang="en"]').waitFor()
  } finally {
    await page.close()
  }
}, { timeout: 60_000 })

test('compact Top5 switches retain sorted data, keyboard operation and readable bilingual headers', { timeout: 60_000 }, async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US', viewport: { width: 1280, height: 900 } })
  const fixtures = new Map()
  const fixtureQueries = (nameKey, queries) => Object.values(queries).map((query, index) => {
    const values = [[7, 31, 18], [42, 9, 26], [14, 6, 21], [1, 33, 12]][index]
    const data = values.map((value, position) => ({
      metric: { [nameKey]: `${nameKey === 'container_pod_uuid' ? 'worker:ranking-fixture' : nameKey}-${position}` }, value
    }))
    fixtures.set(query, { nameKey, data })
    return query
  })
  const nodeQueries = fixtureQueries('node', buildGroupedResourceTopQueries('node'))
  const deviceQueries = fixtureQueries('device_uuid', buildGroupedResourceTopQueries('device_uuid'))
  const countQueries = Object.entries(buildTaskCountQueries()).map(([key, query]) => fixtureQueries(key === 'byNode' ? 'node' : 'device_uuid', { query })[0])
  const workloadQueries = fixtureQueries('container_pod_uuid', buildTaskAllocationTopQueries())
  const requests = trackMonitorRequests(page)
  await page.route('**/v1/monitor/query/instant-vector', (route) => {
    const fixture = fixtures.get(route.request().postDataJSON()?.query)
    return fixture ? route.fulfill({ json: { code: 0, data: fixture.data } }) : route.continue()
  })
  try {
    for (const [path, queries] of [
      ['overview', [nodeQueries.slice(0, 2), nodeQueries.slice(2)]],
      ['nodes', [nodeQueries.slice(0, 2), nodeQueries.slice(2)]],
      ['accelerators', [deviceQueries.slice(0, 2), deviceQueries.slice(2)]],
      ['workloads', [countQueries, workloadQueries]]
    ]) {
      await page.goto(`${target}${basePath}${path}`, { waitUntil: 'networkidle' })
      const cards = page.locator('.home-block').filter({ has: page.locator('.tab-top-switch') })
      assert.equal(await cards.count(), 2)
      const beforeSwitch = requests.length
      for (const [cardIndex, options] of queries.entries()) {
        const card = cards.nth(cardIndex)
        const buttons = card.locator('.tab-top-switch button')
        assert.equal(await buttons.count(), options.length)
        for (const [index, query] of options.entries()) {
          await buttons.nth(index).press(index % 2 ? 'Space' : 'Enter')
          assert.equal(await buttons.nth(index).getAttribute('aria-pressed'), 'true')
          const fixture = fixtures.get(query)
          const expected = fixture.data.slice().sort((left, right) => right.value - left.value)
          const rows = card.locator('.tab-top-item')
          await waitUntil(async() => await rows.count() === expected.length, 'Top5 switch did not show its data')
          assert.deepEqual(await rows.locator('.tab-top-value').evaluateAll((elements) => elements.map((element) => Number(element.textContent.match(/[\d.]+/)[0]))), expected.map((item) => item.value))
          for (const [position, item] of expected.entries()) {
            const name = await rows.nth(position).evaluate((element) => element.querySelector('.ranking-workload-link')?.getAttribute('aria-label') || element.querySelector('.tab-top-name')?.getAttribute('title'))
            assert.ok(name.includes(item.metric[fixture.nameKey].split(':').at(-1)), `${path}: switching metrics changed value-to-name ordering: ${JSON.stringify({ cardIndex, index, position, name, expected: item.metric[fixture.nameKey] })}`)
          }
        }
      }
      assert.equal(requests.length, beforeSwitch, 'Switching between loaded Top5 views must not issue monitoring requests')
      const originalLanguage = await page.locator('html').getAttribute('lang')
      for (const language of [originalLanguage, originalLanguage === 'en' ? 'zh-CN' : 'en']) {
        if (await page.locator('html').getAttribute('lang') !== language) {
          await page.getByRole('button', { name: language === 'en' ? 'English' : '中文', exact: true }).click()
          await page.waitForLoadState('networkidle')
        }
        for (const width of [1280, 1366]) {
          await page.setViewportSize({ width, height: 900 })
          const headers = await cards.locator('.home-block-header').evaluateAll((elements) => elements.map((header) => {
            const title = header.querySelector('.title')
            const group = header.querySelector('.tab-top-switch')
            const titleBox = title.getBoundingClientRect()
            const groupBox = group.getBoundingClientRect()
            const box = header.getBoundingClientRect()
            return {
              title: title.textContent.trim(), label: group.getAttribute('aria-label'), height: groupBox.height,
              titleFits: title.scrollWidth <= title.clientWidth + 1,
              groupFits: groupBox.left >= box.left && groupBox.right <= box.right + 1,
              separated: groupBox.top >= titleBox.bottom || titleBox.top >= groupBox.bottom || groupBox.left >= titleBox.right,
              fontSizes: [...group.querySelectorAll('button')].map((button) => getComputedStyle(button).fontSize)
            }
          }))
          for (const header of headers) {
            assert.ok(header.titleFits && header.groupFits && header.separated, `${path} ${language} ${width}: clipped Top5 header ${JSON.stringify(header)}`)
            assert.equal(header.label, header.title)
            assert.equal(header.height, 30)
            assert.ok(header.fontSizes.every((size) => size === '13px'))
          }
          if (path === 'overview' || path === 'workloads') {
            await cards.first().scrollIntoViewIfNeeded()
            await captureTrendScreenshot(page, `p7-${path}-${language}-${width}-compact-top5`)
          }
        }
      }
    }
  } finally {
    await page.close()
  }
})

test('workload rankings show Pod and container names independently of list filters', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US', viewport: { width: 1440, height: 1000 } })
  const podName = 'distributed-training-worker-with-a-long-identifiable-pod-name'
  const workloads = [
    { name: 'main', appName: podName, podUid: 'pod-research', namespace: 'research' },
    { name: 'main', appName: podName, podUid: 'pod-production', namespace: 'production' },
    { name: 'worker', appName: 'worker', podUid: 'pod-worker', namespace: 'default' }
  ].map((item) => ({ ...item, nodeName: 'node-1', status: 'success', deviceIds: ['gpu-1'], createTime: '2026-09-12T00:00:00Z' }))
  let filteredRequests = 0
  await page.route(workloadListPattern, (route) => {
    const name = route.request().postDataJSON()?.filters?.name
    if (name) filteredRequests += 1
    const items = name ? workloads.filter((item) => item.name === name) : workloads
    return fulfillWorkloadFixture(route, { code: 0, items, total: items.length })
  })
  await page.route('**/api/vgpu/v1/monitor/query/instant-vector', (route) => {
    if (!route.request().postDataJSON()?.query?.includes('container_pod_uuid')) return route.continue()
    return route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ code: 0, data: [
        ...workloads.map((item, index) => ({
          metric: { container_pod_uuid: `${item.name}:${item.podUid}` }, value: 1 - index * 0.2
        })),
        { metric: { container_pod_uuid: 'main:pod-no-longer-in-inventory' }, value: 0.1 }
      ] })
    })
  })

  try {
    await page.goto(`${target}${basePath}workloads`, { waitUntil: 'domcontentloaded' })
    const rankings = page.locator('.ranking-workload')
    await rankings.first().getByRole('link', { name: `${podName} / main`, exact: true }).waitFor()
    assert.equal(await rankings.count(), 4)
    for (const [index, workload] of workloads.entries()) {
      const row = rankings.nth(index)
      const label = workload.appName === workload.name ? workload.name : `${workload.appName} / ${workload.name}`
      const link = row.getByRole('link', { name: label, exact: true })
      const href = new URL(await link.getAttribute('href'), target)
      assert.equal(href.pathname, `${basePath}workloads/${workload.podUid}/containers/${workload.name}`)
      assert.match(await row.locator('.ranking-namespace').textContent(), new RegExp(workload.namespace))
    }
    assert.equal(await rankings.nth(2).locator('.ranking-pod-name').count(), 0)
    const fallback = rankings.nth(3)
    await fallback.getByRole('link', { name: 'main / Pod UID pod-no-longer-in-inventory', exact: true }).waitFor()
    assert.match(await fallback.textContent(), /Pod UID/)
    const firstRankingLink = rankings.first().getByRole('link')
    await firstRankingLink.hover()
    const identityLayout = await rankings.first().evaluate((element) => {
      const label = element.querySelector('.ranking-workload-label').getBoundingClientRect()
      const namespace = element.querySelector('.ranking-namespace').getBoundingClientRect()
      const decoration = getComputedStyle(element.querySelector('.ranking-workload-label'), '::after')
      return {
        topDelta: Math.abs(label.top - namespace.top),
        decorationBottom: decoration.bottom,
        decorationHeight: decoration.height,
        decorationOpacity: decoration.opacity,
        decorationColor: decoration.backgroundColor,
        linkColor: getComputedStyle(element.querySelector('a')).color
      }
    })
    assert.ok(identityLayout.topDelta <= 0.5, JSON.stringify(identityLayout))
    assert.equal(identityLayout.decorationBottom, '0px')
    assert.equal(identityLayout.decorationHeight, '1px')
    assert.equal(identityLayout.decorationOpacity, '1')
    assert.equal(identityLayout.decorationColor, identityLayout.linkColor)
    await rankings.first().locator('.ranking-pod-name').hover()
    const tooltip = page.locator('[role="tooltip"]').filter({ hasText: podName }).last()
    await tooltip.waitFor({ state: 'visible' })
    assert.equal((await tooltip.textContent()).trim(), podName)

    const search = page.getByRole('textbox', { name: 'Search Pod or container name', exact: true })
    await search.fill('worker')
    await search.press('Enter')
    await waitUntil(() => filteredRequests > 0, 'Workload name filter did not issue a request')
    await page.locator('.workload-table .workload-identity-link[aria-label="worker"]').waitFor()
    await waitUntil(
      async() => await page.locator('.workload-table .workload-identity-link').count() === 1,
      'Filtered workload rows did not replace the previous list'
    )
    assert.equal(await page.locator('.workload-table .workload-identity-link').count(), 1)
    assert.equal(await rankings.count(), 4)
    await rankings.first().getByRole('link', { name: `${podName} / main`, exact: true }).waitFor()
    const dimensions = await page.evaluate(() => ({
      document: document.documentElement.scrollWidth, viewport: window.innerWidth
    }))
    assert.ok(dimensions.document <= dimensions.viewport, JSON.stringify(dimensions))
    await rankings.nth(1).getByRole('link').click()
    await page.waitForURL((url) => url.pathname.endsWith('/workloads/pod-production/containers/main'))
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
  const fulfill = (route, items, status = 200) => fulfillWorkloadFixture(route, status === 200
      ? { code: 0, items, total: Array.isArray(items) ? items.length : 0 }
      : { code: status, message: 'temporary list failure' }, status)
  const workload = (name) => ({
    name,
    appName: '',
    podUid: `pod-${name}`,
    nodeName: 'node-1',
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

  await page.route(workloadListPattern, async(route) => {
    const handler = responses.shift()
    assert.ok(handler, 'Workload list issued an unexpected request')
    receivedRequests += 1
    await handler(route)
    completedRequests += 1
  })

  const initialGate = createGate()
  // The table and the unfiltered ranking inventory each load once.
  // Both requests share the same response so their arrival order is irrelevant.
  const initialResponse = async(route) => {
    await initialGate.promise
    await fulfill(route, { invalid: true })
  }
  enqueue(initialResponse)
  enqueue(initialResponse)

  try {
    await page.goto(
      `${target}${basePath}workloads`,
      { waitUntil: 'domcontentloaded' }
    )
    await page.locator('[data-testid="stateful-table-skeleton"]').waitFor()
    assert.equal(
      await page.locator('.stateful-table').getAttribute('aria-busy'),
      'true'
    )

    await waitUntil(() => receivedRequests === 2, 'Initial table and ranking inventory requests did not start')
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

    const refreshButton = page.locator('.table-toolbar-right').getByRole('button', { name: 'Refresh', exact: true })
    const refreshIcon = await refreshButton.locator('.refresh-button__icon').elementHandle()
    enqueue((route) => fulfill(route, [workload('stable-worker')]))
    await refreshButton.click()
    await page.locator('.workload-table .ellipsis-text')
      .filter({ hasText: 'stable-worker' })
      .waitFor()

    const tableOffsetFromToolbar = () => page.locator('.workload-table').evaluate((table) => (
      table.getBoundingClientRect().top -
      document.querySelector('.table-toolbar').getBoundingClientRect().bottom
    ))
    const readyTableOffset = await tableOffsetFromToolbar()
    const assertTablePosition = async() => {
      const offset = await tableOffsetFromToolbar()
      assert.ok(
        Math.abs(offset - readyTableOffset) < 0.5,
        `Refreshing moved the table: ${readyTableOffset}px to ${offset}px below the toolbar`
      )
    }

    const failedRefreshGate = createGate()
    enqueue(async(route) => {
      await failedRefreshGate.promise
      await fulfill(route, [], 503)
    })
    await refreshButton.click()
    await page.locator('[data-testid="stateful-table-refreshing"]').waitFor({ state: 'attached' })
    assert.equal(await page.locator('.stateful-table').getAttribute('aria-busy'), 'true')
    assert.equal(await refreshButton.isEnabled(), true)
    await assertRefreshAnimation(page, refreshButton, refreshIcon)
    await assertTablePosition()
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
    await assertTablePosition()

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
    await assertTablePosition()

    enqueue((route) => fulfill(route, [workload('newest-worker')]))
    await refreshButton.click()
    await page.locator('.workload-table .ellipsis-text')
      .filter({ hasText: 'newest-worker' })
      .waitFor()
    await assertTablePosition()

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
        `${target}${basePath}accelerators/gpu-delayed`,
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
      `${basePath}accelerators/gpu-missing`
    ))
    await t.test('task', () => assertMissingDetail(
      target,
      `${basePath}workloads/pod-missing/containers/missing-worker`
    ))
  })

  await t.test('a failed node detail can be retried', async() => {
    const requestKey = 'node:node-retry'
    const initialAttempts = backendRequestCounts.get(requestKey) ?? 0
    const page = await browser.newPage()
    try {
      await page.goto(
        `${target}${basePath}nodes/node-retry?nodeName=node-retry`,
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
        `${target}${basePath}accelerators/gpu-metric-old`,
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
          '/accelerators/gpu-metric-new'
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

test('trend pages request each series once on mount and once when refreshing a preset', async(t) => {
  const target = await startWebEntry({ frameAncestors: undefined })
  for (const [route, expectedCount] of trendPages) {
    await t.test(route, async() => {
      const page = await browser.newPage({ locale: 'en-US' })
      await page.clock.install({ time: Date.now() })
      const requests = trackTrendRequests(page)
      try {
        await page.goto(`${target}${basePath}${route}`, { waitUntil: 'networkidle' })
        assert.equal(requests.length, expectedCount, 'Mount must preserve the parent range')
        const initialEnd = requests[0].range.end
        await page.clock.setSystemTime(Date.now() + 61_000)
        await selectTrendRange(page, '1h')
        await page.waitForLoadState('networkidle')
        assert.equal(requests.length, expectedCount * 2, 'Reselecting a preset must publish once')
        assert.notEqual(requests.at(-1).range.end, initialEnd)

        await selectTrendRange(page, '3h')
        await page.waitForLoadState('networkidle')
        assert.equal(requests.length, expectedCount * 3, 'Changing presets must publish once')

        const selected = trendRangeButton(page, '3h')
        for (const [index, key] of ['Enter', 'Space'].entries()) {
          await page.clock.setSystemTime(Date.now() + 120_000 + index * 61_000)
          await selected.press(key)
          await page.waitForLoadState('networkidle')
          assert.equal(requests.length, expectedCount * (4 + index), `${key} must refresh once`)
        }
      } finally {
        await page.close()
      }
    })
  }
}, { timeout: 60_000 })

test('explicit trend refresh advances relative ranges once and blocks repeat requests on every trend page', { timeout: 60_000 }, async(t) => {
  const target = await startWebEntry({ frameAncestors: undefined })
  for (const [route, seriesCount] of trendPages) {
    await t.test(route, async() => {
      const page = await browser.newPage({ locale: 'en-US', viewport: { width: 1440, height: 900 }, timezoneId: 'UTC' })
      await page.clock.install({ time: new Date('2026-10-07T12:00:00Z') })
      const requests = trackTrendRequests(page)
      const held = []
      let hold = true
      try {
        await page.goto(`${target}${basePath}${route}`, { waitUntil: 'networkidle' })
        const refresh = page.getByRole('button', { name: 'Refresh trends', exact: true })
        const refreshIcon = await refresh.locator('.refresh-button__icon').elementHandle()
        assert.equal(requests.length, seriesCount)
        assert.equal(await refresh.isEnabled(), true)
        await page.route('**/v1/monitor/query/range-vector', async(route) => {
          if (hold) await new Promise((release) => held.push(release))
          await route.continue()
        })
        await page.clock.setSystemTime(new Date('2026-10-07T12:01:01Z'))
        await refresh.click()
        await waitUntil(() => held.length === seriesCount, 'Refresh did not start exactly one batch of trend requests')
        assert.equal(requests.length, seriesCount * 2)
        assert.equal(await refresh.isDisabled(), true, 'Refresh must be disabled while trend requests are pending')
        assert.equal(await refresh.getAttribute('aria-busy'), 'true')
        if (route === 'overview') await assertRefreshAnimation(page, refresh, refreshIcon)
        for (const request of requests.slice(seriesCount)) {
          assert.deepEqual([request.range.start, request.range.end], ['2026-10-07 11:01:01', '2026-10-07 12:01:01'])
        }
        const bounds = await refresh.boundingBox()
        await page.mouse.click(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2)
        await page.keyboard.press('Enter')
        await page.keyboard.press('Space')
        assert.equal(requests.length, seriesCount * 2, 'Mouse and keyboard activation while busy must not start another batch')
        hold = false
        held.splice(0).forEach((release) => release())
        await waitUntil(() => refresh.isEnabled(), 'Refresh did not become available after every trend request completed')
        await page.waitForLoadState('networkidle')
        assert.equal(await refresh.isEnabled(), true)
        for (const [index, key] of ['Enter', 'Space'].entries()) {
          await page.clock.setSystemTime(new Date(`2026-10-07T12:0${index + 2}:02Z`))
          if (key === 'Enter') {
            await refresh.focus()
            await page.keyboard.down(key)
            await waitUntil(() => requests.length === seriesCount * (3 + index), 'Keyboard refresh did not request one batch')
            await waitUntil(() => refresh.isEnabled(), 'The first Enter refresh must finish before testing key repeat')
            await page.waitForLoadState('networkidle')
            await refresh.focus()
            await page.keyboard.down(key)
            await page.keyboard.up(key)
          } else {
            await refresh.press(key)
          }
          await page.waitForLoadState('networkidle')
          assert.equal(requests.length, seriesCount * (3 + index), `${key} must refresh exactly once, including a held key after the response`)
          assert.equal(requests.at(-1).range.end, `2026-10-07 12:0${index + 2}:02`)
          assert.equal(await trendRangeButton(page, '1h').getAttribute('aria-pressed'), 'true')
        }
      } finally {
        hold = false
        held.splice(0).forEach((release) => release())
        await page.close()
      }
    })
  }
})

test('explicit custom refresh cancels drafts, preserves applied dates and can retry after failure', { timeout: 60_000 }, async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US', viewport: { width: 1440, height: 900 }, timezoneId: 'UTC' })
  await page.clock.install({ time: new Date('2026-10-07T12:00:00Z') })
  const requests = trackTrendRequests(page)
  try {
    await page.goto(`${target}${deepRoute}`, { waitUntil: 'networkidle' })
    const filter = page.locator('.trend-time-filter')
    const refresh = filter.getByRole('button', { name: 'Refresh trends', exact: true })
    await selectTrendRange(page, 'custom')
    const start = filter.getByRole('textbox', { name: 'Start Time', exact: true })
    const end = filter.getByRole('textbox', { name: 'End Time', exact: true })
    const applied = ['2026-10-03 10:12:34', '2026-10-06 11:23:45']
    await start.click()
    await start.fill(applied[0])
    await end.click()
    await end.fill(applied[1])
    await end.press('Enter')
    await page.waitForLoadState('networkidle')
    assert.equal(requests.length, 10)
    for (const width of [960, 1000, 1024]) {
      await filter.evaluate(async(element, width) => {
        element.style.width = `${width}px`
        await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)))
      }, width)
      assert.ok(await customTimestampFieldsFit(filter), `Full timestamps must remain readable at the ${width}px container boundary`)
      assert.equal(requests.length, 10, 'Resizing custom controls must not query')
    }
    await filter.evaluate((element) => { element.style.width = '' })

    await page.clock.setSystemTime(new Date('2026-10-07T12:05:00Z'))
    await start.click()
    await start.fill('2026-10-02 01:02:03')
    await refresh.click()
    await page.locator('.trend-time-filter-popup:visible').waitFor({ state: 'hidden' })
    await page.waitForLoadState('networkidle')
    assert.equal(requests.length, 15, 'Refreshing a custom range must request once without applying the draft')
    assert.deepEqual([await start.inputValue(), await end.inputValue()], applied)
    assert.equal(await trendRangeButton(page, 'custom').getAttribute('aria-pressed'), 'true')
    assert.equal(await page.locator('.t-message').count(), 0)
    for (const request of requests.slice(10)) assert.deepEqual([request.range.start, request.range.end], applied)

    let fail = true
    await page.route('**/v1/monitor/query/range-vector', (route) => fail
      ? route.fulfill({ status: 500, body: 'injected refresh failure' })
      : route.continue())
    await refresh.press('Enter')
    await page.locator('.metric-chart__refresh--error').first().waitFor()
    await page.waitForLoadState('networkidle')
    assert.equal(requests.length, 20)
    assert.equal(await refresh.isEnabled(), true, 'A failed refresh must allow another attempt')
    fail = false
    await refresh.press('Space')
    await page.waitForLoadState('networkidle')
    assert.equal(requests.length, 25, 'Retry must request exactly one batch')
    assert.equal(await page.locator('.metric-chart__refresh--error').count(), 0)
    await waitUntil(() => refresh.isEnabled(), 'A completed retry must enable refresh again')
    assert.deepEqual([await start.inputValue(), await end.inputValue()], applied)
    for (const request of requests.slice(15)) assert.deepEqual([request.range.start, request.range.end], applied)
    await captureTrendScreenshot(page, 'p7-custom-refresh-applied')
  } finally {
    await page.close()
  }
})

test('responsive trend controls fit all trend pages without fetching on resize, language or sidebar changes', { timeout: 120_000 }, async(t) => {
  const target = await startWebEntry({ frameAncestors: undefined })
  for (const [route, expectedCount] of trendPages) {
    await t.test(route, async() => {
      const page = await browser.newPage({ locale: 'en-US', viewport: { width: 1280, height: 900 } })
      const requests = trackTrendRequests(page)
      try {
        await page.goto(`${target}${basePath}${route}`, { waitUntil: 'networkidle' })
        const filter = page.locator('.trend-time-filter')
        for (const language of ['en', 'zh-CN']) {
          if (language === 'zh-CN') await page.getByRole('button', { name: '中文', exact: true }).click()
          await page.locator(`html[lang="${language}"]`).waitFor()
          for (const width of [1280, 1366, 1440, 1920]) {
            await page.setViewportSize({ width, height: 900 })
            for (const collapsed of [false, true]) {
              if (collapsed) await page.getByRole('button', { name: language === 'en' ? 'Collapse sidebar' : '收起侧栏', exact: true }).click()
              await page.locator('.page-aside').evaluate(async(element) => {
                await Promise.all(element.getAnimations().map((animation) => animation.finished))
              })
              const context = `${route}, ${language}, ${width}px, ${collapsed ? 'collapsed' : 'expanded'}`
              const layout = await filter.evaluate((element) => {
                const rect = (node) => {
                  const { x, y, width, height, right, bottom } = node.getBoundingClientRect()
                  return { x, y, width, height, right, bottom }
                }
                const group = element.querySelector('.trend-time-filter-presets')
                const select = element.querySelector('.trend-time-filter-select')
                const visible = (node) => node && node.getBoundingClientRect().width > 0
                const buttons = [...group.querySelectorAll('.segmented-control__option')]
                const selected = group.querySelector('[aria-pressed="true"]')
                const indicator = group.querySelector('.segmented-control__indicator')
                const refresh = element.querySelector('.trend-time-filter-refresh')
                return {
                  ...rect(element), clientWidth: element.clientWidth, scrollWidth: element.scrollWidth,
                  group: { ...rect(group), visible: visible(group), background: getComputedStyle(group).backgroundColor },
                  selectVisible: visible(select),
                  refresh: { ...rect(refresh), label: refresh.getAttribute('aria-label') },
                  buttons: buttons.map((button) => ({ ...rect(button), fontSize: getComputedStyle(button).fontSize, labelFits: button.scrollWidth <= button.clientWidth + 1 })),
                  selectedBackground: getComputedStyle(indicator).backgroundColor,
                  selected: buttons.indexOf(selected)
                }
              })
              assert.ok(layout.scrollWidth <= layout.clientWidth + 1 && layout.right <= width + 1, `${context}: selector overflows`)
              assert.notEqual(layout.group.visible, layout.selectVisible, `${context}: exactly one preset entry must be visible`)
              assert.equal(layout.selected, 0)
              assert.ok(layout.refresh.width >= 28 && Math.abs(layout.refresh.right - layout.right) <= 1, `${context}: refresh must remain at the right edge`)
              assert.equal(layout.refresh.label, language === 'en' ? 'Refresh trends' : '刷新趋势')
              if (layout.group.visible) {
                assert.equal(layout.buttons.length, 7)
                assert.notEqual(layout.group.background, 'rgba(0, 0, 0, 0)', `${context}: presets need a filled background`)
                assert.notEqual(layout.group.background, layout.selectedBackground, `${context}: selected range needs a distinct fill`)
                assert.equal(layout.selectedBackground, 'rgb(255, 255, 255)')
                for (const button of layout.buttons) {
                  assert.ok(button.x >= layout.x - 1 && button.right <= layout.right + 1 && button.labelFits, `${context}: a preset is clipped or requires horizontal scrolling`)
                  assert.ok(button.height >= 24 && button.height <= 32, `${context}: presets must retain the shared segmented-control height`)
                  assert.equal(button.fontSize, '14px', `${context}: labels must not shrink to fit`)
                }
              }
              assert.equal(requests.length, expectedCount, `${context}: presentation changes must not fetch trends`)
              if (route === 'overview' && ((language === 'en' && width === 1280 && !collapsed) || (language === 'zh-CN' && width === 1920 && collapsed))) {
                await captureTrendScreenshot(page, `p7-${language}-${width}-${collapsed ? 'collapsed' : 'expanded'}`)
              }
              if (collapsed) await page.getByRole('button', { name: language === 'en' ? 'Expand sidebar' : '展开侧栏', exact: true }).click()
            }
          }
        }
        await page.waitForLoadState('networkidle')
        assert.equal(requests.length, expectedCount, 'The complete layout matrix must retain the applied range')
      } finally {
        await page.close()
      }
    })
  }
})

test('narrow trend presets preserve selection, keyboard focus and exactly one refresh per activation', { timeout: 60_000 }, async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US', viewport: { width: 1440, height: 900 }, timezoneId: 'UTC' })
  await page.clock.install({ time: new Date('2026-10-07T12:00:00Z') })
  const requests = trackTrendRequests(page)
  try {
    await page.goto(`${target}${deepRoute}`, { waitUntil: 'networkidle' })
    const filter = page.locator('.trend-time-filter')
    const selected = trendRangeButton(page, '1h')
    await selected.focus()
    await filter.evaluate((element) => { element.style.width = '600px' })
    const narrow = filter.locator('.trend-time-filter-select')
    const input = narrow.getByRole('textbox', { name: 'Time range', exact: true })
    await narrow.waitFor()
    await waitUntil(async() => await narrow.evaluate((element) => element.contains(document.activeElement)), 'Narrow mode must retain focus on the visible preset entry')
    assert.equal(requests.length, 5)
    await filter.evaluate((element) => { element.style.width = '900px' })
    await selected.waitFor()
    await waitUntil(async() => await selected.evaluate((element) => element === document.activeElement || element.contains(document.activeElement)), 'Wide mode must return focus to the selected preset')
    assert.equal(requests.length, 5)
    await filter.evaluate((element) => { element.style.width = '360px' })
    await narrow.waitFor()
    await input.press('Enter')
    const options = page.locator('.t-select-option:visible')
    await options.first().waitFor()
    await stablePopupBounds(page.locator('.t-popup:visible').filter({ has: page.locator('.t-select-option') }))
    await captureTrendScreenshot(page, 'p7-en-360-preset-menu')
    assert.deepEqual((await options.allTextContents()).map((label) => label.trim()), (await filter.locator('.segmented-control__option').allTextContents()).map((label) => label.trim()))
    assert.equal(requests.length, 5, 'Opening presets must not refresh')
    await input.press('Escape')
    await options.first().waitFor({ state: 'hidden' })
    assert.equal(requests.length, 5, 'Escape must not refresh')
    assert.ok(await narrow.evaluate((element) => element.contains(document.activeElement)))

    await input.press('Space')
    await options.first().waitFor()
    for (let index = 0; index < 4; index += 1) await input.press('ArrowDown')
    await input.press('Enter')
    await page.waitForLoadState('networkidle')
    assert.equal(requests.length, 10, 'A keyboard selection must publish once')
    assert.equal(new Date(requests.at(-1).range.end) - new Date(requests.at(-1).range.start), 24 * 60 * 60 * 1000)

    const initialEnd = requests.at(-1).range.end
    await page.clock.setSystemTime(new Date('2026-10-07T12:01:01Z'))
    await selectTrendRange(page, '24h')
    await page.waitForLoadState('networkidle')
    assert.equal(requests.length, 15, 'Reselecting the current dropdown item must refresh once')
    assert.notEqual(requests.at(-1).range.end, initialEnd)
    await filter.evaluate((element) => { element.style.width = '900px' })
    await trendRangeButton(page, '24h').waitFor()
    await waitUntil(async() => await trendRangeButton(page, '24h').evaluate((element) => element === document.activeElement), 'Returning to wide mode must focus the current non-default preset')
    await filter.evaluate((element) => { element.style.width = '360px' })
    await narrow.waitFor()
    await waitUntil(async() => await narrow.evaluate((element) => element.contains(document.activeElement)), 'Returning to narrow mode must preserve selector focus')
    await narrow.click()
    await options.first().waitFor()
    await input.press('Tab')
    await page.keyboard.press('Tab')
    await filter.evaluate((element) => { element.style.width = '900px' })
    await options.first().waitFor({ state: 'hidden' })
    assert.equal(await filter.evaluate((element) => element.contains(document.activeElement)), false, 'Closing a hidden dropdown after Tab must not pull focus back')
    assert.equal(requests.length, 15, 'Hiding an open dropdown must not select an option')
    await filter.evaluate((element) => { element.style.width = '360px' })
    await narrow.waitFor()
    await narrow.click()
    await options.first().waitFor()
    await page.locator('.home-page-title').click()
    await options.first().waitFor({ state: 'hidden' })
    assert.equal(requests.length, 15, 'Outside dismissal must not refresh')
    await filter.evaluate((element) => { element.style.width = '900px' })
    await trendRangeButton(page, '24h').waitFor()
    assert.equal(await trendRangeButton(page, '24h').getAttribute('aria-pressed'), 'true')
    assert.equal(await filter.evaluate((element) => element.contains(document.activeElement)), false, 'Resizing after clicking the page title must not steal focus back')
    assert.equal(requests.length, 15, 'Returning to wide mode must preserve the selected range')
    await captureTrendScreenshot(page, 'p7-preset-keyboard-wide')
    await filter.evaluate((element) => { element.style.width = '360px' })
    await narrow.waitFor()
    const refresh = filter.getByRole('button', { name: 'Refresh trends', exact: true })
    const [filterBounds, refreshBounds] = await Promise.all([filter.boundingBox(), refresh.boundingBox()])
    assert.ok(Math.abs(refreshBounds.x + refreshBounds.width - filterBounds.x - filterBounds.width) <= 1, 'Compact refresh must stay at the right edge')
    await page.clock.setSystemTime(new Date('2026-10-07T12:02:02Z'))
    await refresh.click()
    await page.waitForLoadState('networkidle')
    assert.equal(requests.length, 20, 'Compact refresh must publish exactly once')
    assert.deepEqual([requests.at(-1).range.start, requests.at(-1).range.end], ['2026-10-06 12:02:02', '2026-10-07 12:02:02'])
  } finally {
    await page.close()
  }
})

test('custom trend drafts survive resizing and narrow calendars keep confirmation reachable', { timeout: 60_000 }, async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US', viewport: { width: 1440, height: 900 }, timezoneId: 'UTC' })
  await page.clock.install({ time: new Date('2026-10-07T12:00:00Z') })
  const requests = trackTrendRequests(page)
  try {
    await page.goto(`${target}${deepRoute}`, { waitUntil: 'networkidle' })
    await page.getByRole('button', { name: 'Collapse sidebar', exact: true }).click()
    const filter = page.locator('.trend-time-filter')
    await selectTrendRange(page, 'custom')
    let start = filter.getByRole('textbox', { name: 'Start Time', exact: true })
    let end = filter.getByRole('textbox', { name: 'End Time', exact: true })
    const applied = [await start.inputValue(), await end.inputValue()]
    await start.click()
    const startElement = await start.elementHandle()
    const rawDraft = '2026-10-03 10:12:'
    await start.fill(rawDraft)
    for (const width of [600, 360, 900, 360]) {
      await filter.evaluate(async(element, width) => {
        element.style.width = `${width}px`
        await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)))
      }, width)
      assert.ok(await startElement.evaluate((element) => element.isConnected && element === document.activeElement), 'Resizing must keep the same focused date input')
      assert.equal(await start.inputValue(), rawDraft, 'Resizing must retain incomplete text exactly')
      assert.equal(await end.inputValue(), applied[1])
      assert.equal(requests.length, 5)
    }
    await page.getByRole('button', { name: 'Switch to 中文', exact: true }).click()
    await page.locator('html[lang="zh-CN"]').waitFor()
    start = filter.getByRole('textbox', { name: '开始时间', exact: true })
    end = filter.getByRole('textbox', { name: '结束时间', exact: true })
    await waitUntil(async() => await start.inputValue() === applied[0], 'Clicking the language button must cancel an unconfirmed draft')
    assert.deepEqual([await start.inputValue(), await end.inputValue()], applied)
    assert.equal(requests.length, 5, 'Changing language must not apply the canceled draft')

    await start.click()
    await start.fill(rawDraft)
    await filter.evaluate((element) => { element.style.width = '100%' })
    const panel = page.locator('.trend-time-filter-popup:visible')
    for (const width of [390, 1440, 375]) {
      await page.setViewportSize({ width, height: 844 })
      await stablePopupBounds(panel)
      assert.equal(await start.inputValue(), rawDraft, 'Changing calendar height must not turn scroll anchoring into a time edit')
      assert.ok(await start.evaluate((element) => element === document.activeElement), 'Viewport changes must not steal date-input focus')
    }
    const selected = ['2026-10-03 10:12:34', '2026-10-06 11:23:45']
    await start.fill(selected[0])
    await end.click()
    await end.fill(selected[1])
    await panel.waitFor()
    await panel.getByText('本地时间 · UTC', { exact: true }).waitFor()
    const confirm = panel.getByRole('button', { name: '确定', exact: true })
    for (const [width, height] of [[390, 844], [375, 667]]) {
      await page.setViewportSize({ width, height })
      const bounds = await stablePopupBounds(panel)
      await captureTrendScreenshot(page, `p7-zh-${width}-calendar-bounds`)
      assert.ok(bounds.x >= 0 && bounds.y >= 0 && bounds.x + bounds.width <= width + 1 && bounds.y + bounds.height <= height + 1, `Calendar leaves ${width}x${height} viewport: ${JSON.stringify(bounds)}`)
      await confirm.scrollIntoViewIfNeeded()
      assert.ok(await confirm.evaluate((element) => {
        const rect = element.getBoundingClientRect()
        return rect.x >= 0 && rect.y >= 0 && rect.right <= innerWidth && rect.bottom <= innerHeight &&
          element.contains(document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2))
      }), `Calendar confirmation must be visible and unobstructed at ${width}x${height}`)
    }
    assert.equal(requests.length, 5, 'Narrow calendar drafts must not submit early')
    await confirm.click()
    await panel.waitFor({ state: 'hidden' })
    await page.waitForLoadState('networkidle')
    assert.deepEqual([requests.at(-1).range.start, requests.at(-1).range.end], selected)
    assert.equal(requests.length, 10, 'Narrow confirmation must apply exactly once')
    assert.ok(await end.evaluate((element) => element === document.activeElement))
    await waitUntil(async() => await end.evaluate((element) => getComputedStyle(element.closest('.t-input')).backgroundColor) === 'rgb(242, 243, 255)', 'Narrow focused input must retain the accepted light-blue fill')
    assert.ok(await customTimestampFieldsFit(filter), 'Narrow fields must show complete timestamps without horizontal scrolling')
    await captureTrendScreenshot(page, 'p7-zh-375-custom-applied')

    await end.click()
    await panel.waitFor()
    const beforeWheel = await end.inputValue()
    await panel.locator('.t-time-picker__panel-body-scroll').nth(1).hover()
    await page.mouse.wheel(0, 60)
    await waitUntil(async() => await end.inputValue() !== beforeWheel, 'Real mouse wheel must still edit the time')
    const afterWheel = await end.inputValue()
    assert.ok(Number.isFinite(new Date(afterWheel).getTime()))
    assert.equal(requests.length, 10, 'Wheel editing must not submit before confirmation')
    await confirm.click()
    await waitUntil(() => requests.length === 15, 'Confirming a wheel edit must fetch exactly once')
    assert.equal(requests.at(-1).range.end, afterWheel)
  } finally {
    await page.close()
  }
})

test('workload status segments keep same-selection no-op behavior when shared controls emit activation', { timeout: 60_000 }, async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US' })
  const requests = []
  page.on('request', (request) => {
    if (new URL(request.url()).pathname.endsWith('/v1/workloads')) requests.push(request.postDataJSON())
  })
  try {
    await page.goto(`${target}${basePath}workloads`, { waitUntil: 'networkidle' })
    const segments = page.locator('.workload-status-filter')
    const selected = segments.locator('[aria-pressed="true"]')
    const initialCount = requests.length
    assert.ok(initialCount > 0)
    await selected.click()
    await selected.press('Enter')
    await selected.press('Space')
    await page.waitForLoadState('networkidle')
    assert.equal(requests.length, initialCount, 'Activating the current workload status must remain a no-op')
    const next = segments.locator('[aria-pressed="false"]').first()
    await next.click()
    await page.waitForLoadState('networkidle')
    assert.equal(requests.length, initialCount + 1, 'Changing the workload status must request once')
    assert.ok(requests.at(-1).filters.status)
    await segments.locator('[aria-pressed="true"]').click()
    await page.waitForLoadState('networkidle')
    assert.equal(requests.length, initialCount + 1)
  } finally {
    await page.close()
  }
})

test('custom trend dates apply exact input and restore the applied range on cancellation or invalid input', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US', viewport: { width: 1366, height: 900 }, timezoneId: 'America/Argentina/Buenos_Aires' })
  const requests = []
  page.on('request', (request) => {
    if (request.url().endsWith('/v1/monitor/query/range-vector')) requests.push(request.postDataJSON())
  })
  try {
    await page.goto(`${target}${deepRoute}`, { waitUntil: 'networkidle' })
    const filter = page.locator('.trend-time-filter')
    await selectTrendRange(page, 'custom')
    const start = filter.getByRole('textbox', { name: 'Start Time', exact: true })
    const end = filter.getByRole('textbox', { name: 'End Time', exact: true })
    assert.equal(await start.getAttribute('aria-label'), 'Start Time')
    assert.equal(await end.getAttribute('aria-label'), 'End Time')
    assert.ok(await start.inputValue())
    assert.ok(await end.inputValue())
    assert.equal(requests.length, 5)

    const inputBackground = (input) => input.evaluate((element) => getComputedStyle(element.closest('.t-input')).backgroundColor)
    const title = page.locator('.home-page-title')
    const restingBackgrounds = await Promise.all([inputBackground(start), inputBackground(end)])
    for (const [index, input] of [start, end].entries()) {
      await input.hover()
      await waitUntil(async() => await inputBackground(input) === 'rgb(242, 243, 255)', 'An enabled date input must use a light brand hover background')
      await title.hover()
      await waitUntil(async() => await inputBackground(input) === restingBackgrounds[index], 'Leaving an unfocused date input must restore its background')
    }
    await start.click()
    const panel = page.locator('.trend-time-filter-popup:visible')
    const timezone = await page.evaluate(() => Intl.DateTimeFormat().resolvedOptions().timeZone)
    const timezoneNote = panel.getByText(`Local time · ${timezone}`, { exact: true })
    await timezoneNote.waitFor()
    await page.setViewportSize({ width: 375, height: 667 })
    await stablePopupBounds(panel)
    const [noteBounds, confirmBounds] = await Promise.all([
      timezoneNote.boundingBox(),
      panel.getByRole('button', { name: 'Confirm', exact: true }).boundingBox()
    ])
    assert.ok(noteBounds.x >= 0 && noteBounds.y >= 0 && noteBounds.x + noteBounds.width <= 375 && noteBounds.y + noteBounds.height <= 667, 'The complete long timezone note must remain inside the narrow viewport')
    assert.ok(noteBounds.x + noteBounds.width <= confirmBounds.x || noteBounds.y + noteBounds.height <= confirmBounds.y, 'The timezone note must not overlap calendar confirmation')
    assert.ok(await panel.getByRole('button', { name: 'Confirm', exact: true }).evaluate((element) => {
      const rect = element.getBoundingClientRect()
      return rect.x >= 0 && rect.y >= 0 && rect.right <= innerWidth && rect.bottom <= innerHeight &&
        element.contains(document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2))
    }), 'Long timezone text must leave confirmation visible and unobstructed')
    await page.setViewportSize({ width: 1366, height: 900 })
    await stablePopupBounds(panel)
    await waitUntil(async() => await inputBackground(start) === 'rgb(242, 243, 255)', 'The focused start input must not look disabled')
    const originalStart = await start.inputValue()
    await start.press('ControlOrMeta+A')
    assert.deepEqual(await start.evaluate((element) => [element.selectionStart, element.selectionEnd]), [0, originalStart.length])
    await start.press('ArrowRight')
    await start.press('Backspace')
    await start.press(originalStart.at(-1))
    assert.equal(await start.inputValue(), originalStart, 'Keyboard editing must preserve the entered timestamp')
    await title.hover()
    await start.press('Tab')
    assert.ok(await end.evaluate((element) => element === document.activeElement), 'Tab must move from start to end')
    await waitUntil(async() => await inputBackground(end) === 'rgb(242, 243, 255)', 'The focused end input must not look disabled')
    await end.press('Tab')
    assert.ok(await filter.getByRole('button', { name: 'Refresh trends', exact: true }).evaluate((element) => element === document.activeElement), 'Tab from the end input must go directly to refresh')
    await title.click()
    await waitUntil(async() => JSON.stringify(await Promise.all([inputBackground(start), inputBackground(end)])) === JSON.stringify(restingBackgrounds), 'Blurring the picker must restore both input backgrounds')
    assert.equal(requests.length, 5, 'Hover, focus, selection, and unchanged typing must not request another range')

    await start.click()
    await end.press('Enter')
    await page.waitForLoadState('networkidle')
    assert.equal(requests.length, 5, 'Confirming unchanged visible seconds must not refetch')

    const selected = ['2025-09-01 10:12:34', '2025-09-01 11:23:45']
    await start.click()
    await start.fill(selected[0])
    await end.click()
    await end.fill(selected[1])
    await end.press('Enter')
    await page.waitForLoadState('networkidle')
    assert.deepEqual([await start.inputValue(), await end.inputValue()], selected)
    assert.equal(requests.length, 10)
    assert.equal(await page.locator('.t-message').count(), 0, 'Valid Enter must not show a warning')
    assert.deepEqual(
      [requests.at(-1).range.start, requests.at(-1).range.end],
      selected
    )

    await start.click()
    await start.fill('2025-08-01 01:02:03')
    await page.locator('.home-page-title').click()
    await waitUntil(async() => await start.inputValue() === selected[0], 'Cancel did not restore the applied start')
    assert.deepEqual([await start.inputValue(), await end.inputValue()], selected)
    assert.equal(requests.length, 10)

    await start.fill('')
    await page.locator('.home-page-title').click()
    assert.equal(await start.inputValue(), selected[0])
    assert.equal(requests.length, 10)

    await end.fill('2999-01-01 00:00:00')
    await end.press('Enter')
    await page.locator('.t-message').filter({ hasText: 'End time cannot be in the future. The previous range has been restored.' }).waitFor()
    assert.deepEqual([await start.inputValue(), await end.inputValue()], selected)
    assert.equal(requests.length, 10)

    await start.fill('2025-10-01 00:00:00')
    await end.press('Enter')
    await page.locator('.t-message').filter({ hasText: 'End time must be after start time. The previous range has been restored.' }).waitFor()
    assert.deepEqual([await start.inputValue(), await end.inputValue()], selected)
    assert.equal(requests.length, 10)

    await selectTrendRange(page, '3h')
    await page.waitForLoadState('networkidle')
    assert.equal(await page.locator('.t-message').count(), 0, 'Changing presets must clear the range warning')
    assert.equal(await filter.getByRole('status').textContent(), '')
    await selectTrendRange(page, 'custom')
    assert.deepEqual(
      [await start.inputValue(), await end.inputValue()],
      [requests.at(-1).range.start, requests.at(-1).range.end]
    )
    assert.equal(requests.length, 15)

    for (const locale of ['en', 'zh-CN']) {
      if (locale === 'zh-CN') {
        await page.getByRole('button', { name: '中文', exact: true }).click()
        await page.locator('html[lang="zh-CN"]').waitFor()
        assert.equal(await filter.getByRole('textbox', { name: '开始时间', exact: true }).getAttribute('aria-label'), '开始时间')
        assert.equal(await filter.getByRole('textbox', { name: '结束时间', exact: true }).getAttribute('aria-label'), '结束时间')
      }
      for (const width of [1280, 1366, 1440]) {
        await page.setViewportSize({ width, height: 900 })
        const dimensions = await filter.evaluate((element) => ({
          width: element.clientWidth,
          content: element.scrollWidth,
          right: element.getBoundingClientRect().right
        }))
        assert.ok(dimensions.content <= dimensions.width + 1, `${locale} filter overflows at ${width}`)
        assert.ok(dimensions.right <= width, `${locale} filter leaves the viewport at ${width}`)
      }
    }
  } finally {
    await page.close()
  }
}, { timeout: 60_000 })

test('custom trend Enter rejects empty and malformed input with one warning and preserves keyboard cancellation', async(t) => {
  const target = await startWebEntry({ frameAncestors: undefined })
  for (const name of ['empty start', 'empty end', 'malformed start']) {
    await t.test(name, async() => {
      const page = await browser.newPage({ locale: 'en-US' })
      await page.clock.install({ time: new Date('2026-10-06T12:00:00.123Z') })
      const requests = []
      page.on('request', (request) => {
        if (request.url().endsWith('/v1/monitor/query/range-vector')) requests.push(request.postDataJSON())
      })
      try {
        await page.goto(`${target}${deepRoute}`, { waitUntil: 'networkidle' })
        const filter = page.locator('.trend-time-filter')
        await selectTrendRange(page, 'custom')
        const start = filter.getByRole('textbox', { name: 'Start Time', exact: true })
        const end = filter.getByRole('textbox', { name: 'End Time', exact: true })
        const applied = [await start.inputValue(), await end.inputValue()]
        assert.equal(requests.length, 5)
        await page.clock.setSystemTime(new Date('2026-10-06T12:01:01.123Z'))

        const input = name === 'empty end' ? end : start
        await input.click()
        const panel = page.locator('.t-date-range-picker__panel-container:visible')
        await panel.waitFor()
        await input.fill(name === 'malformed start' ? 'not-a-date' : '')
        await input.press('Enter')
        await panel.waitFor({ state: 'hidden' })
        await page.waitForLoadState('networkidle')
        assert.deepEqual([await start.inputValue(), await end.inputValue()], applied)
        assert.equal(requests.length, 5, 'Invalid Enter must not change the chart range')
        const warning = page.locator('.t-message')
        assert.equal(await warning.count(), 1, 'One Enter must show exactly one warning')
        assert.equal(await warning.textContent(), 'The time range is incomplete or invalid. The previous range has been restored.')
        assert.equal(await filter.getByRole('status').textContent(), await warning.textContent())
        assert.equal(await filter.getByRole('status').getAttribute('aria-atomic'), 'true')
        assert.equal(await filter.locator('.t-is-error, [aria-invalid="true"]').count(), 0, 'Restored valid inputs must not have error styling')

        if (name === 'malformed start') {
          await start.fill('2025-08-01 01:02:03')
          await start.press('Tab')
          await end.press('Tab')
          await waitUntil(async() => await start.inputValue() === applied[0], 'Tabbing away after malformed Enter must discard the next draft')
          assert.deepEqual([await start.inputValue(), await end.inputValue()], applied)
          assert.equal(await panel.count(), 0)
          assert.equal(requests.length, 5)
        }
      } finally {
        await page.close()
      }
    })
  }
}, { timeout: 60_000 })

test('custom trend warnings preserve aligned controls and chart position in both languages', async(t) => {
  const target = await startWebEntry({ frameAncestors: undefined })
  for (const locale of ['en-US', 'zh-CN']) {
    await t.test(locale, async() => {
      const page = await browser.newPage({ locale, viewport: { width: 1440, height: 900 } })
      const requests = []
      page.on('request', (request) => {
        if (request.url().endsWith('/v1/monitor/query/range-vector')) requests.push(request.postDataJSON())
      })
      try {
        await page.goto(`${target}${deepRoute}`, { waitUntil: 'networkidle' })
        const filter = page.locator('.trend-time-filter')
        const chinese = locale === 'zh-CN'
        await selectTrendRange(page, 'custom')
        const start = filter.getByRole('textbox', { name: chinese ? '开始时间' : 'Start Time', exact: true })
        const end = filter.getByRole('textbox', { name: chinese ? '结束时间' : 'End Time', exact: true })
        const applied = [await start.inputValue(), await end.inputValue()]
        const geometry = () => filter.evaluate((element) => {
          const rect = (target) => {
            const bounds = target.getBoundingClientRect()
            return { top: bounds.top + window.scrollY, height: bounds.height }
          }
          return {
            filter: rect(element),
            radio: rect(element.querySelector('.trend-time-filter-presets')),
            picker: rect(element.querySelector('.trend-time-filter-custom')),
            chart: rect(document.querySelector('.home-bottom-row')),
            width: element.clientWidth,
            content: element.scrollWidth,
            right: element.getBoundingClientRect().right
          }
        })
        for (const width of [1280, 1366, 1440]) {
          await page.setViewportSize({ width, height: 900 })
          // ECharts throttles resizing; measure feedback after the surrounding layout settles.
          let previousGeometry
          let stableSince = Date.now()
          const before = await waitUntil(async() => {
            const current = await geometry()
            const currentGeometry = JSON.stringify(current)
            if (currentGeometry !== previousGeometry) {
              previousGeometry = currentGeometry
              stableSince = Date.now()
            }
            return Date.now() - stableSince >= 200 ? current : false
          }, `${locale} layout did not settle at ${width}`)
          if (Math.abs(before.radio.top - before.picker.top) <= 1) {
            assert.ok(Math.abs(before.radio.height - before.picker.height) <= 1, `${locale} control heights differ at ${width}`)
          } else {
            assert.ok(before.picker.top >= before.radio.top + before.radio.height, `${locale} wrapped picker overlaps presets at ${width}`)
          }
          assert.ok(before.content <= before.width + 1, `${locale} filter overflows at ${width}`)
          assert.ok(before.right <= width, `${locale} filter leaves the viewport at ${width}`)

          await end.fill('2999-01-01 00:00:00')
          await end.press('Enter')
          const warning = page.locator('.t-message')
          await warning.waitFor()
          const expected = chinese
            ? '结束时间不能晚于当前时间，已恢复原范围。'
            : 'End time cannot be in the future. The previous range has been restored.'
          assert.equal(await warning.textContent(), expected)
          assert.equal(await warning.count(), 1)
          assert.equal(await filter.getByRole('status').textContent(), expected)
          assert.deepEqual([await start.inputValue(), await end.inputValue()], applied)
          assert.equal(requests.length, 5, 'Rejected edits must not fetch another range')
          assert.equal(await filter.locator('.t-is-error, [aria-invalid="true"], .t-input__tips').count(), 0)
          const after = await geometry()
          assert.equal(after.filter.height, before.filter.height, `${locale} rejection changes filter height at ${width}`)
          assert.equal(after.chart.top, before.chart.top, `${locale} rejection moves charts at ${width}`)
          assert.deepEqual(after.radio, before.radio, `${locale} rejection moves preset buttons at ${width}`)
          assert.deepEqual(after.picker, before.picker, `${locale} rejection moves the picker at ${width}`)

          await end.press('Enter')
          await warning.waitFor({ state: 'detached' })
          assert.equal(await filter.getByRole('status').textContent(), '', 'Accepting the restored range clears feedback')
          assert.equal(requests.length, 5, 'Accepting unchanged seconds must not refetch')
        }
      } finally {
        await page.close()
      }
    })
  }
}, { timeout: 60_000 })

test('custom trend warnings repeat without stacking and ignore composing or repeated Enter', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US' })
  await page.clock.install({ time: Date.now() })
  const requests = []
  page.on('request', (request) => {
    if (request.url().endsWith('/v1/monitor/query/range-vector')) requests.push(request.postDataJSON())
  })
  try {
    await page.goto(`${target}${deepRoute}`, { waitUntil: 'networkidle' })
    const filter = page.locator('.trend-time-filter')
    await selectTrendRange(page, 'custom')
    const start = filter.getByRole('textbox', { name: 'Start Time', exact: true })
    const end = filter.getByRole('textbox', { name: 'End Time', exact: true })
    const applied = [await start.inputValue(), await end.inputValue()]
    const warning = page.locator('.t-message')

    await start.fill('2025-08-01 01:02:03')
    for (const options of [{ isComposing: true }, { keyCode: 229 }, { repeat: true }]) {
      await start.dispatchEvent('keydown', { key: 'Enter', code: 'Enter', ...options })
      assert.equal(await warning.count(), 0)
      assert.equal(requests.length, 5, 'Composition and key repeats must not submit a draft')
      assert.equal(await start.inputValue(), '2025-08-01 01:02:03')
    }
    await page.locator('.home-page-title').click()
    await waitUntil(async() => await start.inputValue() === applied[0], 'Cancel must restore the IME draft')

    for (let attempt = 0; attempt < 2; attempt += 1) {
      await start.fill('not-a-date')
      await start.press('Enter')
      await warning.waitFor()
      assert.equal(await warning.count(), 1, 'Repeating the same rejected input must replace its own warning')
      assert.equal(await filter.getByRole('status').textContent(), 'The time range is incomplete or invalid. The previous range has been restored.')
      assert.deepEqual([await start.inputValue(), await end.inputValue()], applied)
      assert.equal(requests.length, 5)
    }
    await page.clock.fastForward(5500)
    await warning.waitFor({ state: 'detached' })
    await start.fill('not-a-date')
    await start.press('Enter')
    await warning.waitFor()
    assert.equal(await warning.count(), 1, 'The same error must be explained again after the earlier warning expires')
    assert.equal(requests.length, 5)

    await page.getByRole('link', { name: 'Nodes', exact: true }).click()
    await warning.waitFor({ state: 'detached' })
  } finally {
    await page.close()
  }
}, { timeout: 60_000 })

test('custom trend calendar hover preserves continuous range colors and disabled dates', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US', timezoneId: 'UTC' })
  await page.clock.install({ time: new Date('2026-10-07T12:00:00Z') })
  const requests = trackMonitorRequests(page)
  try {
    await page.goto(`${target}${deepRoute}`, { waitUntil: 'networkidle' })
    const filter = page.locator('.trend-time-filter')
    await selectTrendRange(page, 'custom')
    const start = filter.getByRole('textbox', { name: 'Start Time', exact: true })
    const end = filter.getByRole('textbox', { name: 'End Time', exact: true })
    const panel = page.locator('.trend-time-filter-popup:visible')
    const cells = panel.locator('.t-date-picker__cell:not(.t-date-picker__cell--additional)')
    const readCells = () => cells.evaluateAll((elements) => elements.map((cell) => {
      const inner = cell.querySelector('.t-date-picker__cell-inner')
      const { x, y, width, height } = inner.getBoundingClientRect()
      const before = getComputedStyle(cell, '::before')
      const after = getComputedStyle(cell, '::after')
      return {
        day: cell.textContent.trim(), classes: cell.className, hovered: cell.matches(':hover'),
        bounds: { x, y, width, height }, fill: getComputedStyle(inner).backgroundColor,
        outline: getComputedStyle(inner).boxShadow, color: getComputedStyle(inner).color,
        before: before.backgroundColor, after: after.backgroundColor, opacity: after.opacity
      }
    }))
    for (const [range, hovers] of [
      [['2026-10-03 10:00:00', '2026-10-06 11:00:00'], [['start', '2'], ['start', '3'], ['end', '7'], ['end', '8']]],
      [['2026-10-03 10:00:00', '2026-10-03 11:00:00'], [['start', '2']]],
      [['2026-09-29 10:00:00', '2026-10-03 11:00:00'], [['end', '2']]]
    ]) {
      await start.click()
      await start.fill(range[0])
      await end.click()
      await end.fill(range[1])
      await end.press('Enter')
      await page.waitForLoadState('networkidle')
      assert.deepEqual([await start.inputValue(), await end.inputValue()], range)
      const appliedRequestCount = requests.length
      for (const [editing, date] of hovers) {
        await (editing === 'start' ? start : end).click()
        await panel.waitFor()
        await delay(250) // Opening the other input also transitions endpoint colors.
        const resting = await readCells()
        const day = cells.filter({ hasText: new RegExp(`^${date}$`) })
        await day.hover()
        await delay(250) // Let the range opacity and hover fill settle.
        const hovered = await readCells()
        const hoveredDay = hovered.find((cell) => cell.day === date)
        assert.ok(hoveredDay.hovered, 'The pointer must actually hover the requested day')
        assert.deepEqual(hovered.map((cell) => cell.bounds), resting.map((cell) => cell.bounds), 'Hover moved calendar cells')
        const unchanged = (cell) => cell.day !== date || cell.classes.includes('t-date-picker__cell--active') || cell.classes.includes('t-date-picker__cell--disabled')
        assert.deepEqual(hovered.filter(unchanged).map((cell) => cell.fill), resting.filter(unchanged).map((cell) => cell.fill), 'Hover changed an unhovered, selected, or disabled cell fill')
        const preview = hovered.filter((cell) => cell.classes.includes('t-date-picker__cell--hover-highlight'))
        if (date === '8') {
          assert.ok(hoveredDay.classes.includes('t-date-picker__cell--disabled'))
          assert.equal(preview.length, 0, 'A disabled future date must not preview a range')
          await day.click()
          assert.deepEqual([await start.inputValue(), await end.inputValue()], range)
        } else {
          if (!hoveredDay.classes.includes('t-date-picker__cell--active')) {
            assert.equal(hoveredDay.outline, 'none', 'Pointer hover must not draw a focus-like outline')
            assert.equal(hoveredDay.fill, 'rgba(0, 0, 0, 0)', 'A range preview must not add another hover block')
            assert.equal(hoveredDay.color, 'rgb(0, 82, 217)', 'Hover text must use the brand color')
          }
          const overlap = preview.filter((cell) => cell.classes.includes('t-date-picker__cell--highlight'))
          assert.ok(overlap.length > 0, 'The test must overlap a preview and applied range')
          for (const cell of overlap) {
            assert.equal(cell.after, cell.before, `Day ${cell.day} changes the range color at the endpoint`)
            assert.notEqual(cell.after, 'rgba(0, 0, 0, 0)', 'The preview must bridge the endpoint with a solid color')
            assert.equal(cell.opacity, '1')
          }
        }
        assert.equal(requests.length, appliedRequestCount, 'Hover or a disabled click submitted a range')
        await panel.locator('.t-date-picker__header').hover()
        await delay(250)
        assert.deepEqual((await readCells()).map((cell) => cell.fill), resting.map((cell) => cell.fill), 'Leaving the calendar must restore the resting fills')
        await page.locator('.home-page-title').click()
        await panel.waitFor({ state: 'hidden' })
        assert.deepEqual([await start.inputValue(), await end.inputValue()], range)
      }
    }
  } finally {
    await page.close()
  }
}, { timeout: 60_000 })

test('calendar confirmation applies one range and canceled calendar edits keep it', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US', viewport: { width: 1280, height: 900 } })
  const requests = []
  await page.clock.install({ time: new Date('2026-10-06T12:00:00.123Z') })
  page.on('request', (request) => {
    if (request.url().endsWith('/v1/monitor/query/range-vector')) requests.push(request.postDataJSON())
  })
  try {
    await page.goto(`${target}${deepRoute}`, { waitUntil: 'networkidle' })
    const filter = page.locator('.trend-time-filter')
    await selectTrendRange(page, 'custom')
    const start = filter.getByRole('textbox', { name: 'Start Time', exact: true })
    const end = filter.getByRole('textbox', { name: 'End Time', exact: true })
    await start.click()
    const panel = page.locator('.t-date-range-picker__panel-container:visible')
    const day = (text) => panel.locator('.t-date-picker__cell:not(.t-date-picker__cell--additional)').filter({ hasText: new RegExp(`^${text}$`) })
    assert.ok((await day('7').getAttribute('class')).includes('t-date-picker__cell--disabled'))
    await day('1').click()
    await end.click()
    await day('2').click()
    await panel.locator('.t-time-picker__panel-body-scroll').first().getByText('04', { exact: true }).click()
    await waitUntil(async() => (await end.inputValue()).includes(' 04:'), 'Time column selection did not update the draft')
    assert.equal(requests.length, 5, 'Calendar drafts must not fetch before confirmation')
    const selected = [await start.inputValue(), await end.inputValue()]
    assert.ok(selected[0].startsWith('2026-10-01 '), JSON.stringify(selected))
    assert.ok(selected[1].startsWith('2026-10-02 '), JSON.stringify(selected))
    await panel.getByRole('button', { name: 'Confirm', exact: true }).click()
    await page.waitForLoadState('networkidle')
    assert.equal(requests.length, 10)
    assert.deepEqual([requests.at(-1).range.start, requests.at(-1).range.end], selected)
    assert.equal(await panel.count(), 0, 'Confirm must finish editing and close the calendar')
    assert.ok(await end.evaluate((element) => element === document.activeElement), 'Confirm must return keyboard focus to the date input')

    await start.click()
    await panel.getByRole('button', { name: 'Confirm', exact: true }).click()
    await waitUntil(async() => await panel.count() === 0, 'Confirming an unchanged range must also close the calendar')
    assert.equal(requests.length, 10, 'Confirming the applied range must not fetch again')

    await start.fill('2025-08-01 01:02:03')
    await start.press('Tab')
    await end.press('Tab')
    await waitUntil(async() => await start.inputValue() === selected[0], 'Tabbing away after Confirm must discard unconfirmed input')
    assert.deepEqual([await start.inputValue(), await end.inputValue()], selected)
    assert.equal(await panel.count(), 0)
    assert.equal(requests.length, 10)

    await start.click()
    await end.click()
    await day('3').click()
    const nextSelected = [await start.inputValue(), await end.inputValue()]
    assert.ok(nextSelected[1].startsWith('2026-10-03 '), JSON.stringify(nextSelected))
    await panel.getByRole('button', { name: 'Confirm', exact: true }).click()
    await page.waitForLoadState('networkidle')
    assert.equal(requests.length, 15, 'A new calendar range after Confirm must apply once')
    assert.deepEqual([requests.at(-1).range.start, requests.at(-1).range.end], nextSelected)
    assert.equal(await panel.count(), 0)

    await start.click()
    await day('4').click()
    await page.locator('.home-page-title').click()
    await waitUntil(async() => await start.inputValue() === nextSelected[0], 'Calendar cancellation did not restore the applied range')
    assert.deepEqual([await start.inputValue(), await end.inputValue()], nextSelected)
    assert.equal(requests.length, 15)
  } finally {
    await page.close()
  }
}, { timeout: 60_000 })

test('workload status labels stay concise while accessible help explains container evidence', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US' })
  const statuses = ['waiting', 'success', 'not_ready', 'error', 'closed', 'failed', 'terminating', 'unknown']
  const details = {
    waiting: { containerState: 'Waiting', reason: 'ContainerCreating', ready: false, restartCount: 0, podPhase: 'Pending' },
    success: { containerState: 'Running', ready: true, restartCount: 0, podReady: 'True', podPhase: 'Running' },
    recovered: { containerState: 'Running', ready: true, restartCount: 3, podReady: 'False', lastTerminationReason: 'OOMKilled', lastExitCode: 137 },
    not_ready: { containerState: 'Running', ready: false, restartCount: 0, podPhase: 'Running', podReady: 'False', podReadyReason: 'ContainersNotReady', podReadyMessage: 'worker is not ready' },
    error: { containerState: 'Waiting', reason: 'ImagePullBackOff', message: 'registry returned <unauthorized>\n'.repeat(100), ready: false, restartCount: 0 },
    crashloop: { containerState: 'Waiting', reason: 'CrashLoopBackOff', lastTerminationReason: 'Error', lastExitCode: 42, restartCount: 5, message: 'Back-off restarting failed container main in pod webui-demo-crashloop', podReadyReason: 'ContainersNotReady', podReadyMessage: 'containers with unready status: [main]' },
    closed: { containerState: 'Terminated', reason: 'Completed', exitCode: 0, restartCount: 0, podPhase: 'Succeeded' },
    failed: { containerState: 'Terminated', reason: 'Error', exitCode: 1, restartCount: 0, podPhase: 'Failed' },
    terminating: { containerState: 'Running', ready: true, restartCount: 0, podPhase: 'Running' },
    unknown: { podPhase: 'Unknown', podReady: 'Unknown' },
  }
  const workloads = [...statuses, 'crashloop', 'legacy', 'recovered'].map((code) => ({
    name: `worker-${code}`,
    appName: `pod-${code}`,
    podUid: `uid-${code}`,
    namespace: 'default',
    nodeName: 'node-1',
    nodeUid: 'node-1',
    status: code === 'legacy' ? 'failed' : code === 'recovered' ? 'success' : code === 'crashloop' ? 'error' : code,
    ...(code === 'legacy' ? {} : { statusDetail: details[code] }),
    deviceIds: ['gpu-1'],
    allocatedDevices: 1,
    allocatedCores: 100,
    allocatedMem: 1024,
    createTime: '2026-08-31T00:00:00Z',
  }))
  const fulfill = (route, payload) => fulfillWorkloadFixture(route, { code: 0, ...payload })
  const requestedStatuses = []
  await page.route(workloadListPattern, (route) => {
    const status = route.request().postDataJSON()?.filters?.status
    requestedStatuses.push(status)
    const items = status === 'abnormal'
      ? workloads.filter((item) => ['not_ready', 'error', 'failed', 'unknown'].includes(item.status))
      : status ? workloads.filter((item) => item.status === status) : workloads
    const statusCounts = { all: workloads.length }
    for (const [key, codes] of Object.entries({ waiting: ['waiting'], success: ['success'], abnormal: ['not_ready', 'error', 'failed', 'unknown'] })) {
      statusCounts[key] = workloads.filter((item) => codes.includes(item.status)).length
    }
    return fulfill(route, { items, statusCounts })
  })
  await page.route('**/api/vgpu/v1/container?*', (route) => {
    const params = new URL(route.request().url()).searchParams
    const workload = workloads.find((item) => item.name === params.get('name') && item.podUid === params.get('podUid'))
    assert.ok(workload, 'Unexpected workload details identity')
    return fulfill(route, workload)
  })

  try {
    await page.goto(`${target}${basePath}workloads`, { waitUntil: 'domcontentloaded' })
    await page.locator('.workload-table [data-workload-status="not_ready"]').waitFor()
    assert.deepEqual(
      (await page.locator('.workload-table .workload-status__label').allTextContents()).map((value) => value.trim()),
      ['Starting', 'Running', 'Abnormal', 'Abnormal', 'Completed', 'Abnormal', 'Terminating', 'Abnormal', 'Abnormal', 'Abnormal']
    )
    // The recovered workload is on page two with the default ten-row page size.
    assert.equal(await page.locator('.workload-table .workload-status .metric-help').count(), 8)
    const assertRunningAppearance = async(status, expectedTextColor) => {
      const appearance = await status.evaluate((element) => {
        const icon = element.querySelector('.workload-status__icon')
        const use = icon?.querySelector('use')
        const box = icon?.getBoundingClientRect()
        return {
          icon: use?.getAttribute('href') || use?.getAttribute('xlink:href'),
          size: [box?.width, box?.height],
          textColor: getComputedStyle(element.querySelector('.workload-status__label')).color,
        }
      })
      assert.deepEqual(appearance, {
        icon: '#icon-status-schedulable', size: [16, 16], textColor: expectedTextColor,
      })
    }
    const healthyStatus = page.locator('.workload-table [data-workload-status="success"]').first()
    await assertRunningAppearance(healthyStatus, 'rgb(0, 0, 0)')
    assert.equal(await healthyStatus.locator('.metric-help').count(), 0)
    assert.equal(await page.locator('.workload-table [data-workload-status="closed"] .metric-help').count(), 0)
    for (const code of ['not_ready', 'error', 'failed', 'unknown']) {
      const icon = page.locator(`.workload-table [data-workload-status="${code}"] .workload-status__icon use`).first()
      assert.equal(await icon.evaluate((element) => element.getAttribute('href') || element.getAttribute('xlink:href')), '#icon-status-unschedulable')
    }

    const help = page.locator('.workload-table [data-workload-status="not_ready"] .metric-help')
    const tooltip = page.locator('.t-tooltip .t-popup__content').filter({ hasText: 'Started, but not ready yet.' }).last()
    await help.hover()
    await tooltip.waitFor({ state: 'visible' })
    assert.equal((await tooltip.textContent()).trim(), 'Started, but not ready yet.')
    assert.equal(await tooltip.evaluate((element) => getComputedStyle(element).whiteSpace), 'pre-line')
    assert.ok((await tooltip.boundingBox()).width <= 320)
    await help.focus()
    await help.press('Escape')
    await tooltip.waitFor({ state: 'hidden' })
    await page.mouse.move(0, 0)
    await page.getByPlaceholder('Search Pod or container name').focus()
    await help.focus()
    await tooltip.waitFor({ state: 'visible' })
    await help.press('Escape')
    await tooltip.waitFor({ state: 'hidden' })

    const imageErrorHelp = page.locator('.workload-table [data-workload-status="error"] .metric-help').first()
    await imageErrorHelp.focus()
    const errorTooltip = page.locator('.t-tooltip .t-popup__content').filter({ hasText: 'Image pull failed; retrying.' }).last()
    await errorTooltip.waitFor({ state: 'visible' })
    assert.equal((await errorTooltip.textContent()).trim(), 'Image pull failed; retrying.')
    assert.equal(await errorTooltip.locator('unauthorized').count(), 0)
    assert.ok((await errorTooltip.boundingBox()).height <= 80)
    await imageErrorHelp.press('Escape')

    const crashHelp = page.locator('.workload-table [data-workload-status="error"] .metric-help').nth(1)
    await crashHelp.focus()
    const crashTooltip = page.locator('.t-tooltip .t-popup__content').filter({ hasText: 'The program keeps exiting; retrying.' }).last()
    await crashTooltip.waitFor({ state: 'visible' })
    assert.equal((await crashTooltip.textContent()).trim(), 'The program keeps exiting; retrying.\nLast exit code: 42\nRestart count: 5')
    assert.ok((await crashTooltip.boundingBox()).height <= 120)
    await crashHelp.press('Escape')

    const statusTabs = page.getByRole('group', { name: 'Status', exact: true })
    const tabLabels = async() => (await statusTabs.getByRole('button').allTextContents()).map((label) => label.replace(/\s+/g, ' ').trim())
    await waitUntil(async() => (await tabLabels()).at(-1) === 'Abnormal 6', 'Status tabs did not show the four lifecycle filters with counts')
    assert.deepEqual(await tabLabels(), ['All 11', 'Pending 0', 'Starting 1', 'Running 2', 'Abnormal 6'])
    assert.equal(await statusTabs.getByRole('button', { name: /^All/ }).getAttribute('aria-pressed'), 'true')
    await statusTabs.getByRole('button', { name: /^Abnormal/ }).click()
    await waitUntil(async() => await page.locator('.workload-table .workload-status').count() === 6, 'Abnormal group did not include all error, failed, not-ready and unconfirmed containers')
    assert.equal(requestedStatuses.at(-1), 'abnormal')
    assert.deepEqual(await page.locator('.workload-table .workload-status').evaluateAll((elements) => elements.map((element) => element.dataset.workloadStatus)), ['not_ready', 'error', 'failed', 'unknown', 'error', 'failed'])

    for (const [label, filter, codes] of [
      ['Starting', 'waiting', ['waiting']],
      ['Running', 'success', ['success', 'success']],
      ['All', undefined, workloads.slice(0, 10).map((item) => item.status)],
    ]) {
      await statusTabs.getByRole('button', { name: new RegExp(`^${label}\\b`) }).click()
      assert.equal(await statusTabs.getByRole('button', { name: new RegExp(`^${label}\\b`) }).getAttribute('aria-pressed'), 'true')
      await waitUntil(async() => await page.locator('.workload-table .workload-status').count() === codes.length, `${label} filter returned unexpected workloads`)
      assert.equal(requestedStatuses.at(-1), filter)
      assert.deepEqual(await page.locator('.workload-table .workload-status').evaluateAll((elements) => elements.map((element) => element.dataset.workloadStatus)), codes)
    }

    // Opening a workload and returning keeps the selected status tab.
    await statusTabs.getByRole('button', { name: /^Running/ }).click()
    await waitUntil(() => new URL(page.url()).searchParams.get('status') === 'success', 'The selected status was not kept in the address')
    await page.locator('.workload-table').getByRole('link', { name: 'pod-success / worker-success', exact: true }).click()
    await page.waitForURL((url) => url.pathname.endsWith('/workloads/uid-success/containers/worker-success'))
    const requestsBeforeReturn = requestedStatuses.length
    await page.goBack({ waitUntil: 'domcontentloaded' })
    await waitUntil(async() => await statusTabs.getByRole('button', { name: /^Running/ }).getAttribute('aria-pressed') === 'true', 'Returning from a workload reset the status tab')
    await waitUntil(() => requestedStatuses.slice(requestsBeforeReturn).includes('success'), 'The restored status was not requested')
    await waitUntil(async() => await page.locator('.workload-table .workload-status').count() === 2, 'The restored status did not filter the table')

    await page.goto(
      `${target}${basePath}workloads/uid-success/containers/worker-success`,
      { waitUntil: 'domcontentloaded' }
    )
    const headerStatus = page.locator('.layout-header-title-run-state .workload-status')
    await headerStatus.waitFor()
    assert.equal((await headerStatus.locator('.workload-status__label').textContent()).trim(), 'Running')
    await assertRunningAppearance(headerStatus, 'rgb(50, 69, 88)')
    assert.equal(await headerStatus.locator('.metric-help').count(), 0)

    await page.goto(
      `${target}${basePath}workloads/uid-recovered/containers/worker-recovered`,
      { waitUntil: 'domcontentloaded' }
    )
    await headerStatus.getByRole('button', { name: 'View workload status details' }).focus()
    const recoveredTooltip = page.locator('.t-tooltip .t-popup__content').filter({ hasText: 'This container is ready, but the overall Pod is not ready.' }).last()
    await recoveredTooltip.waitFor({ state: 'visible' })
    assert.equal((await recoveredTooltip.textContent()).trim(), 'This container is ready, but the overall Pod is not ready.\nLast exit code: 137\nRestart count: 3')

    await page.goto(
      `${target}${basePath}workloads/uid-legacy/containers/worker-legacy`,
      { waitUntil: 'domcontentloaded' }
    )
    await page.locator('.layout-header-title-run-state [data-workload-status="failed"]').waitFor()
    await page.locator('.layout-header-title-run-state .metric-help').focus()
    await page.locator('.t-tooltip .t-popup__content')
      .filter({ hasText: 'Reported as Abnormal; no further details.' })
      .last().waitFor({ state: 'visible' })
  } finally {
    await page.close()
  }
}, { timeout: 60_000 })

test('distribution legends keep each name and its count on one line', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US', viewport: { width: 1280, height: 800 } })
  const legends = {
    'count by (device_type)': [['NVIDIA-A100-SXM4-80GB', 59], ['Ascend910B3', 42], ['NVIDIA-H100-SXM5-80GB', 24], ['DCU', 12345]],
    'count by (provider)': [['Ascend', 13], ['NVIDIA', 11], ['DCU', 1], ['HCU', 1]],
  }
  await page.route('**/monitor/query/instant-vector', async(route) => {
    const query = route.request().postDataJSON()?.query || ''
    const prefix = Object.keys(legends).find((key) => query.startsWith(key))
    if (!prefix) return route.fallback()
    const label = prefix.includes('provider') ? 'provider' : 'device_type'
    await route.fulfill({
      json: { code: 0, data: legends[prefix].map(([name, value]) => ({ metric: { [label]: name }, value })) }
    })
  })
  const legendRows = () => page.locator('.nodeCard-legend li').evaluateAll((items) => items.map((item) => {
    const text = item.querySelector('.ellipsis-text')
    const range = document.createRange()
    range.selectNodeContents(text)
    const name = text.getBoundingClientRect()
    const row = item.getBoundingClientRect()
    const count = item.querySelector('.legend-count')
    const countBox = count.getBoundingClientRect()
    return {
      name: text.textContent.trim(),
      height: row.height,
      sameLine: Math.abs(name.top + name.bottom - countBox.top - countBox.bottom) < 2,
      countInside: countBox.left >= row.left - 0.5 && countBox.right <= row.right + 0.5,
      countInset: row.right - countBox.right,
      countWhole: count.scrollWidth <= count.clientWidth,
      cut: range.getBoundingClientRect().width - name.width > 0.001,
      focusable: text.getAttribute('tabindex') === '0',
    }
  }))

  try {
    for (const path of ['accelerators', 'nodes']) {
      await page.goto(`${target}${basePath}${path}`, { waitUntil: 'domcontentloaded' })
      // 1334 puts two English Top5 titles of different lengths on either side of the width that fits a title and its switch.
      for (const [width, height] of [[1280, 800], [1334, 768], [1512, 982], [1024, 768]]) {
        await page.setViewportSize({ width, height })
        await page.locator('.nodeCard-legend li').nth(3).waitFor()
        // Only a name the browser cuts takes focus; the check follows the resize.
        await waitUntil(
          async() => (await legendRows()).every((row) => row.cut === row.focusable),
          `${path} ${width}: focus does not follow which names are cut`
        )
        const rows = await legendRows()
        for (const row of rows) {
          assert.ok(row.height <= 18.5 && row.sameLine && row.countInside && row.countWhole, `${path} ${width}: ${JSON.stringify(row)}`)
        }
        const insets = rows.map((row) => row.countInset)
        assert.ok(Math.max(...insets) - Math.min(...insets) < 0.5, `${path} ${width}: counts do not share a right edge`)
        await page.locator('.preview .tab-top-item').nth(1).waitFor()
        const cards = await page.locator('.preview > li').evaluateAll((items) => items.map((item) => ({
          top: Math.round(item.getBoundingClientRect().top),
          header: item.querySelector('.tab-top-switch')?.closest('.home-block-header').getBoundingClientRect().height,
          switchTop: item.querySelector('.tab-top-switch')?.getBoundingClientRect().top,
          firstItemTop: item.querySelector('.tab-top-item')?.getBoundingClientRect().top,
        })))
        if (width === 1512) {
          assert.equal(new Set(cards.map((card) => card.top)).size, 1, `${path} ${width}: the distribution and Top5 cards wrapped`)
        }
        const [, firstTop5, secondTop5] = cards
        if (firstTop5.top === secondTop5.top) {
          assert.ok(
            firstTop5.header === secondTop5.header &&
              Math.abs(firstTop5.switchTop - secondTop5.switchTop) < 0.5 &&
              Math.abs(firstTop5.firstItemTop - secondTop5.firstItemTop) < 0.5,
            `${path} ${width}: Top5 headers side by side differ ${JSON.stringify(cards)}`
          )
        }
      }
    }
  } finally {
    await page.close()
  }
})

test('unconfigured accelerator allocation rates stay unavailable', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US', viewport: { width: 1440, height: 1000 } })
  const device = (uuid, fields) => ({
    uuid, type: 'NVIDIA', nodeName: 'node-1', health: true,
    coreTotal: 100, coreUsed: 0, coreUsedKnown: true, memoryTotal: 24576, memoryUsed: 0, vgpuTotal: 10, vgpuUsed: 0,
    ...fields,
  })
  const devices = [
    device('unc-idle', { type: 'Ascend910', unconfigured: true, memoryTotal: 32768 }),
    device('unc-used', { type: 'Ascend910', unconfigured: true, memoryTotal: 32768, coreUsed: 30, memoryUsed: 8192, vgpuUsed: 1 }),
    device('cfg-idle', {}),
    device('share-unk', { coreUsedKnown: false, memoryUsed: 4096, vgpuUsed: 1 }),
    // The API does not send isExternal today; this guards the existing frontend branch only.
    device('external', { isExternal: true }),
  ]
  await page.route('**/v1/gpus', (route) => route.fulfill({ json: { code: 0, list: devices, total: devices.length } }))
  const rateCells = (uuid) => page.locator('.accelerator-table').evaluate((table, uuid) => {
    const headers = [...table.querySelectorAll('thead th')].map((th) => th.textContent.trim())
    const row = [...table.querySelectorAll('tbody tr')].find((tr) => tr.textContent.includes(uuid))
    const cell = (title) => row?.querySelectorAll('td')[headers.indexOf(title)]
    const read = (td) => ({ text: td?.textContent.trim(), ring: Boolean(td?.querySelector('.t-progress')) })
    return { compute: read(cell('Compute Allocation')), memory: read(cell('Memory Allocation')) }
  }, uuid)

  try {
    await page.goto(`${target}${basePath}accelerators`, { waitUntil: 'domcontentloaded' })
    await page.locator('.accelerator-table tbody tr', { hasText: 'external' }).waitFor()
    const notCounted = { text: '--', ring: false }
    // An unconfigured device has no schedulable capacity, so neither rate reads as a real 0%.
    assert.deepEqual(await rateCells('unc-idle'), { compute: notCounted, memory: notCounted })
    assert.deepEqual(await rateCells('unc-used'), { compute: notCounted, memory: notCounted })
    // A configured device with nothing allocated keeps its real zero.
    assert.deepEqual(await rateCells('cfg-idle'), { compute: { text: '0%', ring: true }, memory: { text: '0%', ring: true } })
    // An unknown compute share hides only the compute rate.
    assert.deepEqual(await rateCells('share-unk'), { compute: notCounted, memory: { text: '16.67%', ring: true } })
    assert.deepEqual(await rateCells('external'), { compute: notCounted, memory: notCounted })
  } finally {
    await page.close()
  }
})

test('overview workload count matches the workload list total', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US', viewport: { width: 1440, height: 1000 } })
  const workloadRequests = []
  let containerRequests = 0
  let reply = 'empty'
  // The old source: 62 containers, which the overview used to count.
  await page.route('**/v1/containers', (route) => {
    containerRequests += 1
    const items = Array.from({ length: 62 }, (_, index) => ({ name: `worker-${index}`, podUid: `pod-${index}`, namespace: 'default' }))
    return route.fulfill({ json: { code: 0, items, total: items.length } })
  })
  const row = { name: 'worker', podUid: 'pod-1', namespace: 'default' }
  await page.route('**/v1/workloads', (route) => {
    workloadRequests.push(route.request().postDataJSON())
    if (reply === 'failed') return route.fulfill({ status: 500, json: { code: 500, message: 'injected failure' } })
    if (reply === 'empty') return route.fulfill({ json: { code: 0, items: [], total: 0, statusCounts: {} } })
    if (reply === 'no total') return route.fulfill({ json: { code: 0, items: [row] } })
    // One row on the page, while the list counts 68 across all pages.
    return route.fulfill({
      json: { code: 0, items: [row], total: 68, statusCounts: { all: 68, pending: 6, success: 60, abnormal: 2 } },
    })
  })
  const workloadCard = page.locator('.resource-overview-item', { hasText: 'Workloads' })
  const cardCount = async() => {
    await workloadCard.locator('.count, .resource-state-text').first().waitFor()
    const shown = await workloadCard.locator('.count').count()
    return shown ? (await workloadCard.locator('.count').textContent()).trim() : null
  }

  try {
    await page.goto(`${target}${basePath}overview`, { waitUntil: 'domcontentloaded' })
    assert.equal(await cardCount(), '0', 'An empty list is a real zero')

    reply = 'no total'
    await page.reload({ waitUntil: 'domcontentloaded' })
    assert.equal(await cardCount(), null, 'A reply without a total must not read as a number')

    reply = 'counted'
    workloadRequests.length = 0
    containerRequests = 0
    await page.reload({ waitUntil: 'domcontentloaded' })
    assert.equal(await cardCount(), '68')
    assert.deepEqual(workloadRequests[0], { filters: {}, page: 1, pageSize: 1 })
    assert.equal(containerRequests, 0, 'The overview still read the container list')

    await workloadCard.click()
    await page.waitForURL((url) => url.pathname.endsWith('/workloads'))
    const listTotal = page.locator('.table-plus-pagination-total')
    await listTotal.waitFor()
    assert.match(await listTotal.textContent(), /\b68\b/, 'The list behind the card counts the same total')

    reply = 'failed'
    await page.goBack({ waitUntil: 'domcontentloaded' })
    await page.waitForURL((url) => url.pathname.endsWith('/overview'))
    assert.equal(await cardCount(), null, 'A failed total must not read as a number')
  } finally {
    await page.close()
  }
})

test('resource names navigate while decorative table icons do not', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US' })
  await page.route(workloadListPattern, (route) => fulfillWorkloadFixture(route, {
      items: [{
        name: 'worker', appName: 'job-1', podUid: 'pod-icon', namespace: 'default', nodeName: 'node-1',
        status: 'success', deviceIds: ['gpu-1'], allocatedCores: 10, allocatedMem: 256,
      }],
      total: 1,
  }))
  const cases = [
    {
      path: 'nodes',
      icon: '.node-table .node-name-icon-card',
      name: '.node-table .text-plus .link',
      detail: 'nodes/node-1?nodeName=node-1',
    },
    {
      path: 'accelerators',
      icon: '.accelerator-table .card-id-cell-icon',
      name: '.accelerator-table .text-plus .link',
      detail: 'accelerators/gpu-1',
    },
    {
      path: 'workloads',
      icon: '.workload-table .task-name-icon-card',
      name: '.workload-table .workload-identity-link',
      detail: 'workloads/pod-icon/containers/worker',
    },
  ]

  try {
    for (const entry of cases) {
      const listURL = `${target}${basePath}${entry.path}`
      await page.goto(listURL, { waitUntil: 'domcontentloaded' })
      const icon = page.locator(entry.icon).first()
      await icon.waitFor()
      await icon.click()
      await delay(100)
      assert.equal(page.url(), listURL, `${entry.path}: a decorative icon navigated`)
      assert.notEqual(await icon.evaluate((element) => getComputedStyle(element).cursor), 'pointer')
      await page.locator(entry.name).first().click()
      await page.waitForURL(`${target}${basePath}${entry.detail}`)
      await page.locator('.detail-page-state[data-detail-state="ready"]').waitFor()
    }
  } finally {
    await page.close()
  }
}, { timeout: 30_000 })

test('earlier addresses redirect to the renamed routes', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US' })
  const cases = [
    ['', 'overview'],
    ['admin/vgpu', 'nodes'],
    ['admin/vgpu/monitor/overview', 'overview'],
    ['admin/vgpu/node/admin?schedulingEligibility=unschedulable', 'nodes?schedulingEligibility=unschedulable'],
    ['admin/vgpu/node/admin/node-1?nodeName=node-1', 'nodes/node-1?nodeName=node-1'],
    ['admin/vgpu/card/admin?type=NVIDIA', 'accelerators?type=NVIDIA'],
    ['admin/vgpu/card/admin/gpu-1', 'accelerators/gpu-1'],
    ['admin/vgpu/task/admin?status=pending', 'workloads?status=pending'],
    ['admin/vgpu/task/admin/detail?name=worker&podUid=pod-icon', 'workloads/pod-icon/containers/worker'],
    ['admin/vgpu/task/admin/detail?name=worker', 'workloads'],
  ]
  try {
    for (const [from, to] of cases) {
      await page.goto(`${target}${basePath}${from}`, { waitUntil: 'domcontentloaded' })
      await page.waitForURL((url) => url.href === `${target}${basePath}${to}`)
      await page.locator('.side-link[aria-current="page"]').waitFor()
    }
  } finally {
    await page.close()
  }
}, { timeout: 30_000 })

test('one workload table combines pending requests and allocations with shared filters and pagination', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US', viewport: { width: 1440, height: 1000 } })
  const makePod = (index) => ({
    name: `pending-${String(index).padStart(2, '0')}`, namespace: 'research', uid: `pending-uid-${index}`,
    createdAt: '2026-09-12T00:00:00Z', schedulerName: 'hami-scheduler', nodeName: '',
    stage: 'waiting', reasonCodes: ['CardInsufficientMemory'], reasonSource: 'hami',
    condition: { status: 'False', reason: 'Unschedulable', message: '1/1 CardInsufficientMemory <b>resource feedback</b>', transitionAt: '2026-09-12T00:00:00Z' },
    requests: [{ container: 'main', containerKind: 'regular', resources: [
      { name: 'nvidia.com/gpu', value: '1', kind: 'count', unit: '' },
      { name: 'nvidia.com/gpucores', value: '1', kind: 'core', unit: '%' },
      { name: 'nvidia.com/gpumem', value: '32768', kind: 'memory', unit: 'MiB' },
    ] }], gates: [], constraints: [], allocatedContainers: [], preallocated: false,
  })
  const pods = Array.from({ length: 23 }, (_, index) => makePod(index))
  pods[1].reasonCodes = ['NodeAffinity']
  pods[1].reasonSource = 'kubernetes'
  pods[2].stage = 'gated'
  pods[2].reasonCodes = ['SchedulingGated']
  pods[2].gates = ['example.com/controller-hold']
  pods[3].schedulerName = 'other-scheduler'
  pods[3].reasonCodes = ['NoFeedback']
  pods[3].condition = null
  const listQueries = []
  let detailRequests = 0
  let detailMode = 'forbidden'
  let releaseDetail
  const firstDetail = new Promise((resolve) => { releaseDetail = resolve })
  const reply = (route, payload, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(payload) })
  const allocatedRow = { name: 'main', appName: 'already-bound', podUid: 'bound-uid', namespace: 'research', nodeName: 'node-1', status: 'error', deviceIds: ['gpu-1'], allocatedCores: 1, allocatedMem: 64 }
  await page.route('**/api/vgpu/v1/containers', (route) => reply(route, { items: [
    allocatedRow,
    { name: 'main', appName: 'only-preallocated', podUid: 'preallocated-uid', namespace: 'research', nodeName: '', status: 'waiting', deviceIds: ['gpu-1'] },
  ] }))
  await page.route('**/api/vgpu/v1/workloads', (route) => {
    const query = route.request().postDataJSON()
    listQueries.push(query)
    const rows = [allocatedRow, ...pods.map(pendingWorkloadFixture)]
    if (detailMode === 'bound') rows[1] = { ...allocatedRow, appName: pods[0].name, podUid: pods[0].uid, status: 'waiting' }
    const filters = query.filters || {}
    const filtered = rows.filter((row) => (!filters.name || row.appName.includes(filters.name) || row.name.includes(filters.name))
      && (!filters.status || row.status === filters.status)
      && (!filters.nodeName || row.nodeName === filters.nodeName))
    return fulfillWorkloadFixture(route, { items: filtered })
  })
  await page.route('**/api/vgpu/v1/scheduling/pod?**', async(route) => {
    detailRequests += 1
    const query = new URL(route.request().url()).searchParams
    assert.equal(query.get('uid'), pods[0].uid)
    assert.equal(query.get('namespace'), 'research')
    if (detailRequests === 1) await firstDetail
    if (detailMode === 'recreated') return reply(route, { reason: 'POD_RECREATED', message: 'Pod was recreated' }, 409)
    if (detailMode === 'gone') return reply(route, { reason: 'POD_NOT_FOUND', message: 'Pod was deleted' }, 404)
    return reply(route, {
      pod: detailMode === 'bound' ? { ...pods[0], nodeName: 'node-1', stage: 'bound', reasonCodes: [], allocatedContainers: ['main'] } : pods[0],
      events: detailMode === 'bound' ? [{ uid: 'old-event', reason: 'FilteringFailed', type: 'Warning', source: 'hami-scheduler', message: 'Historical CardInsufficientMemory <b>literal evidence</b>', lastObservedAt: '2026-09-12T00:00:00Z', count: 17 }] : [],
      eventStatus: detailMode === 'bound' ? 'available' : 'forbidden',
      eventsIncomplete: false, eventsFetchedAt: '2026-09-12T01:00:00Z',
    })
  })
  try {
    await page.goto(`${target}${basePath}workloads`, { waitUntil: 'domcontentloaded' })
    const table = page.locator('.workload-table')
    await table.getByRole('link', { name: 'already-bound / main', exact: true }).waitFor()
    const pendingEntry = table.getByRole('button', { name: 'pending-00 / main', exact: true })
    await pendingEntry.waitFor()
    assert.equal(await page.getByRole('tab').count(), 0, 'Lifecycle states must not split into separate views')
    assert.equal(await page.locator('.table-toolbar').count(), 1)
    assert.equal(await table.getByText('only-preallocated', { exact: true }).count(), 0)
    assert.equal(await page.locator('.task-admin-top-wrap').isVisible(), true)
    assert.equal(await table.locator('tbody tr').count(), 10)
    const rowHeights = await table.locator('tbody tr').evaluateAll((rows) => rows.map((row) => row.getBoundingClientRect().height))
    assert.ok(Math.max(...rowHeights) - Math.min(...rowHeights) < 1, `Mixed lifecycle rows have different heights: ${rowHeights}`)
    const pendingRow = table.locator('tbody tr').filter({ has: page.getByRole('button', { name: 'pending-00 / main', exact: true }) })
    assert.match(await pendingRow.innerText(), /Pending/)
    const resourceValues = await pendingRow.locator('.task-gpu-cell-info > span').allTextContents()
    assert.deepEqual(resourceValues, ['1', '0.01', '32 GiB'])
    assert.doesNotMatch(await pendingRow.locator('.task-gpu-cell').innerText(), /Request|Allocated|Compute|Memory|GPU/)
    const resourceRows = await table.locator('.task-gpu-cell').evaluateAll((cells) => cells.map((cell) => ({
      segments: cell.querySelectorAll('.task-gpu-cell-info > span').length,
      height: cell.getBoundingClientRect().height,
    })))
    assert.ok(resourceRows.every((cell) => cell.segments === 3 && cell.height === resourceRows[0].height))
    assert.equal(detailRequests, 0)
    await pendingRow.getByRole('button', { name: 'View workload status details' }).hover()
    const pendingTooltip = page.locator('.t-tooltip .t-popup__content').filter({ hasText: 'Insufficient allocatable GPU memory' }).last()
    await pendingTooltip.waitFor()
    assert.match(await pendingTooltip.innerText(), /Waiting for (less than a minute|\d+[mhd])/)
    assert.equal(detailRequests, 0, 'Hover or listing must not query Events')
    await page.getByRole('listitem').filter({ hasText: /^2$/ }).click()
    await table.getByRole('button', { name: 'pending-09 / main', exact: true }).waitFor()
    assert.ok(listQueries.some((query) => query.page === 2))
    const search = page.getByRole('textbox', { name: 'Search Pod or container name', exact: true })
    await search.fill('pending-00')
    await search.press('Enter')
    await pendingEntry.waitFor()
    assert.equal(await table.locator('tbody tr').count(), 1)
    assert.equal(listQueries.at(-1).page, 1, 'Applying a filter must reset the shared page')
    await search.fill('')
    await search.press('Enter')
    await table.getByRole('link', { name: 'already-bound / main', exact: true }).waitFor()
    // This backend omits status counts, so the tabs show labels only.
    const statusTabs = page.getByRole('group', { name: 'Status', exact: true })
    assert.deepEqual((await statusTabs.getByRole('button').allTextContents()).map((label) => label.trim()), ['All', 'Pending', 'Starting', 'Running', 'Abnormal'])
    await statusTabs.getByRole('button', { name: 'Pending', exact: true }).click()
    await waitUntil(() => listQueries.at(-1)?.filters?.status === 'pending', 'The shared status filter did not request pending workloads')
    await table.getByRole('link', { name: 'already-bound / main', exact: true }).waitFor({ state: 'hidden' })
    assert.equal(await table.locator('[data-workload-status="pending"]').count(), 10)
    assert.equal(detailRequests, 0, 'Filtering must not read Events')
    await statusTabs.getByRole('button', { name: 'All', exact: true }).click()
    await table.getByRole('link', { name: 'already-bound / main', exact: true }).waitFor()
    await pendingEntry.click()
    const dialog = page.getByRole('dialog', { name: 'Scheduling information', exact: true })
    await dialog.waitFor()
    await waitUntil(() => detailRequests === 1, 'Opening the selected Pod did not request its scheduling detail')
    await dialog.locator('.scheduling-summary').waitFor()
    await dialog.getByRole('button', { name: 'Close scheduling information', exact: true }).waitFor()
    assert.match(await dialog.locator('.scheduling-summary').innerText(), /Insufficient allocatable GPU memory/)
    releaseDetail()
    await dialog.getByText('Event access is not permitted. The Pod scheduling condition is still available.', { exact: true }).waitFor()
    assert.match(await dialog.locator('.scheduling-summary').innerText(), /Insufficient allocatable GPU memory/)
    // Esc first hides a focused help tooltip and leaves the drawer open.
    await dialog.getByRole('button', { name: 'Scheduler', exact: true }).focus()
    await page.locator('.t-tooltip .t-popup__content').filter({ hasText: 'scheduler' }).last().waitFor({ state: 'visible' })
    await page.keyboard.press('Escape')
    await delay(300)
    assert.equal(await dialog.isVisible(), true, 'Esc on a help tooltip closed the drawer')
    detailMode = 'bound'
    await dialog.getByRole('button', { name: 'Refresh scheduling information', exact: true }).click()
    await dialog.locator('.scheduling-summary[data-scheduling-stage="bound"]').waitFor()
    assert.doesNotMatch(await dialog.locator('.scheduling-summary').innerText(), /Insufficient|CardInsufficientMemory/)
    await dialog.getByRole('link', { name: 'View workload details for main', exact: true }).waitFor()
    await table.getByRole('link', { name: 'pending-00 / main', exact: true }).waitFor()
    assert.equal(await table.getByRole('button', { name: 'pending-00 / main', exact: true }).count(), 0, 'Binding must replace the pending row, not add another identity')
    await dialog.locator('summary').filter({ hasText: 'Original scheduling records' }).click()
    await dialog.getByText('Historical CardInsufficientMemory <b>literal evidence</b>', { exact: true }).waitFor()
    assert.equal(await dialog.locator('pre b').count(), 0, 'Original messages must remain text')
    detailMode = 'recreated'
    await dialog.getByRole('button', { name: 'Refresh scheduling information', exact: true }).click()
    await dialog.getByRole('alert').filter({ hasText: 'A new Pod has been created with the same name' }).waitFor()
    assert.equal(await dialog.locator('.scheduling-summary').count(), 0)
    detailMode = 'gone'
    await dialog.getByRole('button', { name: 'Refresh scheduling information', exact: true }).click()
    await dialog.getByRole('alert').filter({ hasText: 'This Pod no longer exists' }).waitFor()
    await page.keyboard.press('Escape')
    await dialog.waitFor({ state: 'hidden' })
    assert.equal(detailRequests, 4)
  } finally {
    releaseDetail()
    await page.close()
  }
}, { timeout: 60_000 })

test('a GPU container assigned without HAMi allocation opens diagnosis with its real startup status', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US' })
  const pod = {
    name: 'native-assigned', namespace: 'research', uid: 'native-assigned-uid',
    schedulerName: 'default-scheduler', nodeName: 'node-1', stage: 'bound', allocatedContainers: [],
    reasonCodes: [], requests: [{ container: 'main', containerKind: 'regular', resources: [
      { name: 'nvidia.com/gpu', kind: 'count', value: '1' },
    ] }],
  }
  const row = {
    ...pendingWorkloadFixture(pod), pending: false, nodeName: 'node-1', status: 'error',
    statusDetail: { containerState: 'Waiting', reason: 'CreateContainerError', ready: false, restartCount: 0, podPhase: 'Pending' },
  }
  let eventRequests = 0
  await page.route('**/api/vgpu/v1/workloads', (route) => fulfillWorkloadFixture(route, { items: [row] }))
  await page.route('**/api/vgpu/v1/scheduling/pod?**', (route) => {
    eventRequests += 1
    assert.equal(new URL(route.request().url()).searchParams.get('uid'), pod.uid)
    return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ pod, events: [], eventStatus: 'empty' }) })
  })
  try {
    await page.goto(`${target}${basePath}workloads`, { waitUntil: 'domcontentloaded' })
    const table = page.locator('.workload-table')
    const entry = table.getByRole('button', { name: 'native-assigned / main', exact: true })
    await entry.waitFor()
    assert.equal(await table.locator('[data-workload-status="error"] .workload-status__label').innerText(), 'Abnormal')
    assert.deepEqual(await table.locator('.task-gpu-cell-info > span').allTextContents(), ['1', '--', '--'])
    assert.equal(eventRequests, 0)
    await entry.click()
    const dialog = page.getByRole('dialog', { name: 'Scheduling information', exact: true })
    await dialog.getByText('Assigned to a node, but no HAMi GPU allocation record was found for this container.', { exact: true }).waitFor()
    await dialog.getByText('default-scheduler', { exact: true }).waitFor()
    assert.equal(await dialog.getByRole('link', { name: /View workload details/ }).count(), 0)
    assert.doesNotMatch(await dialog.locator('.scheduling-summary').innerText(), /Insufficient|previous scheduling failures/)
    await waitUntil(() => eventRequests === 1, 'Opening the assigned container did not request its evidence')
  } finally {
    await page.close()
  }
}, { timeout: 30_000 })

test('node and GPU detail pages do not fetch cluster-wide pending Pods', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US' })
  let schedulingRequests = 0
  page.on('request', (request) => { if (request.url().includes('/v1/scheduling/')) schedulingRequests += 1 })
  try {
    for (const path of ['nodes/node-1?nodeName=node-1', 'accelerators/gpu-1']) {
      await page.goto(`${target}${basePath}${path}`, { waitUntil: 'domcontentloaded' })
      await page.locator('.detail-page-state[data-detail-state="ready"]').waitFor()
      assert.equal(await page.locator('.scheduling-tabs').count(), 0)
      assert.equal(schedulingRequests, 0)
    }
  } finally {
    await page.close()
  }
}, { timeout: 30_000 })

test('closing a scheduling diagnosis prevents its delayed response from replacing another Pod', async() => {
  const target = await startWebEntry({ frameAncestors: undefined })
  const page = await browser.newPage({ locale: 'en-US' })
  const pods = ['first', 'second'].map((name, index) => ({
    name, namespace: 'research', uid: `switch-${index}`, stage: 'waiting',
    schedulerName: 'hami-scheduler', reasonCodes: [index ? 'NodeAffinity' : 'CardInsufficientMemory'],
    reasonSource: index ? 'kubernetes' : 'hami', requests: [{ container: 'main', containerKind: index ? 'sidecar' : 'init', resources: [{ name: 'nvidia.com/gpu', value: '1', kind: 'count' }] }],
  }))
  let releaseFirst
  const delayed = new Promise((resolve) => { releaseFirst = resolve })
  const requested = []
  await page.route('**/api/vgpu/v1/workloads', (route) => fulfillWorkloadFixture(route, { items: pods.map(pendingWorkloadFixture) }))
  await page.route('**/api/vgpu/v1/scheduling/pod?**', async(route) => {
    const uid = new URL(route.request().url()).searchParams.get('uid')
    requested.push(uid)
    if (uid === pods[0].uid) await delayed
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({
      pod: pods.find((pod) => pod.uid === uid), events: [], eventStatus: 'empty',
    }) })
  })
  try {
    await page.goto(`${target}${basePath}workloads`, { waitUntil: 'domcontentloaded' })
    const first = page.getByRole('button', { name: 'first / main', exact: true })
    await first.waitFor()
    const workloadTable = page.locator('.workload-table')
    await workloadTable.getByText('Init container', { exact: true }).waitFor()
    await workloadTable.getByText('Restartable init container', { exact: true }).waitFor()
    const rowHeights = await workloadTable.locator('tbody tr').evaluateAll((rows) => rows.map((row) => row.getBoundingClientRect().height))
    assert.ok(Math.max(...rowHeights) - Math.min(...rowHeights) < 1, `Container kind labels changed row heights: ${rowHeights}`)
    await first.click()
    const dialog = page.getByRole('dialog', { name: 'Scheduling information', exact: true })
    await waitUntil(() => requested.length === 1, 'The first Pod request did not start')
    const close = dialog.getByRole('button', { name: 'Close scheduling information', exact: true })
    await close.focus()
    await page.keyboard.press('Enter')
    await dialog.waitFor({ state: 'hidden' })
    await waitUntil(() => first.evaluate((element) => element === document.activeElement), 'Closing did not restore the Pod entry focus')
    await page.getByRole('button', { name: 'second / main', exact: true }).click()
    await dialog.getByRole('heading', { name: 'second', exact: true }).waitFor()
    await waitUntil(() => requested.length === 2, 'The second Pod request did not start')
    await dialog.getByText('No events found. They may not have been recorded yet or may have expired.', { exact: true }).waitFor()
    releaseFirst()
    await delay(150)
    assert.match(await dialog.locator('.scheduling-summary').innerText(), /Node selection or affinity/)
    assert.doesNotMatch(await dialog.locator('.scheduling-summary').innerText(), /Insufficient allocatable GPU memory/)
    assert.equal(await page.locator('.t-notification').count(), 0, 'A canceled diagnosis must not produce a global error toast')
    assert.deepEqual(requested, ['switch-0', 'switch-1'])
  } finally {
    releaseFirst()
    await page.close()
  }
}, { timeout: 30_000 })
