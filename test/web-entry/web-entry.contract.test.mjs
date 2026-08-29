import assert from 'node:assert/strict'
import http from 'node:http'
import { after, before, test } from 'node:test'
import { setTimeout as delay } from 'node:timers/promises'
import { launchWebEntry } from './launch-web-entry.mjs'

const backendAddress = '127.0.0.1'

let backend
let backendPort
let frontend
let frontendPort
let frontendURL
let frontendLogs = ''
let lastBackendRequest

function withTimeout(promise, timeout, message) {
  let timer
  const timeoutPromise = new Promise((_, reject) => {
    timer = setTimeout(() => reject(new Error(message)), timeout)
    timer.unref?.()
  })

  return Promise.race([promise, timeoutPromise])
    .finally(() => clearTimeout(timer))
}

function captureFrontendLogs(stream) {
  stream.on('data', (chunk) => {
    frontendLogs = `${frontendLogs}${chunk}`.slice(-20_000)
  })
}

async function listen(server, port, host) {
  await new Promise((resolve, reject) => {
    const onError = (error) => reject(error)
    const onListening = () => {
      server.off('error', onError)
      resolve()
    }
    server.once('error', onError)
    if (host) server.listen(port, host, onListening)
    else server.listen(port, onListening)
  })
}

async function closeServer(server) {
  if (!server?.listening) return

  const closed = new Promise((resolve, reject) => {
    server.close((error) => (error ? reject(error) : resolve()))
  })
  try {
    await withTimeout(closed, 2_000, 'Timed out closing the fake backend')
  } catch (error) {
    server.closeAllConnections()
    await withTimeout(closed, 1_000, error.message)
  }
}

async function reservePort(host) {
  const probe = http.createServer()
  await listen(probe, 0, host)
  const address = probe.address()
  assert.equal(typeof address, 'object')
  await closeServer(probe)
  return address.port
}

async function stopProcess(child) {
  if (!child || child.exitCode !== null || child.signalCode !== null) return

  const exited = new Promise((resolve) => child.once(
    'exit',
    (code, signal) => resolve({ code, signal })
  ))
  child.kill('SIGTERM')
  try {
    await withTimeout(exited, 2_000, 'Web entry ignored SIGTERM')
  } catch {
    child.kill('SIGKILL')
    await withTimeout(exited, 2_000, 'Web entry did not exit after SIGKILL')
  }
}

function fetchFrontend(path, options = {}) {
  return fetch(`${frontendURL}${path}`, {
    ...options,
    signal: AbortSignal.timeout(5_000)
  })
}

async function waitForFrontend() {
  const deadline = Date.now() + 15_000
  while (Date.now() < deadline) {
    if (frontend.exitCode !== null || frontend.signalCode !== null) {
      throw new Error(
        `Web entry exited with code ${frontend.exitCode} and signal ` +
        `${frontend.signalCode}.\n${frontendLogs}`
      )
    }

    try {
      const response = await fetch(`${frontendURL}/health_check`, {
        signal: AbortSignal.timeout(500)
      })
      await response.arrayBuffer()
      if (response.ok) {
        await delay(25)
        if (frontend.exitCode === null && frontend.signalCode === null) return
      }
    } catch {
      // The process may still be binding the port.
    }
    await delay(100)
  }

  throw new Error(`Timed out waiting for the Web entry.\n${frontendLogs}`)
}

before(async() => {
  frontendPort = await reservePort(backendAddress)
  do {
    backendPort = await reservePort(backendAddress)
  } while (backendPort === frontendPort)
  frontendURL = `http://${backendAddress}:${frontendPort}`

  backend = http.createServer(async(req, res) => {
    const chunks = []
    for await (const chunk of req) chunks.push(chunk)

    lastBackendRequest = {
      method: req.method,
      url: req.url,
      contentType: req.headers['content-type'],
      probeHeader: req.headers['x-hami-contract-probe'],
      body: Buffer.concat(chunks).toString('utf8')
    }

    if (
      req.url === '/metrics' ||
      req.url === '/readyz' ||
      req.url === '/q/openapi.yaml' ||
      req.url === '/v2/private'
    ) {
      res.writeHead(200, { 'content-type': 'text/plain' })
      res.end('private backend response')
      return
    }

    if (req.url === '/v1/contract-timeout') {
      await delay(500)
      if (!res.destroyed) {
        res.writeHead(200, { 'content-type': 'application/json' })
        res.end(JSON.stringify({ delayed: true }))
      }
      return
    }

    if (req.method !== 'POST' || req.url !== '/v1/nodes?limit=10') {
      res.writeHead(404, { 'content-type': 'application/json' })
      res.end(JSON.stringify({ error: 'unexpected contract-test request' }))
      return
    }

    res.writeHead(400, {
      'content-type': 'application/json',
      'x-upstream-contract': 'preserved'
    })
    res.end(JSON.stringify({ error: 'contract-probe' }))
  })

  frontend = launchWebEntry({
    cwd: process.cwd(),
    listenAddress: `${backendAddress}:${frontendPort}`,
    backendURL: `http://${backendAddress}:${backendPort}`
  })
  captureFrontendLogs(frontend.stdout)
  captureFrontendLogs(frontend.stderr)
  await waitForFrontend()
}, { timeout: 20_000 })

after(async() => {
  await stopProcess(frontend)
  await closeServer(backend)
}, { timeout: 10_000 })

test('serves the SPA shell at the root and an existing deep link', {
  timeout: 10_000
}, async() => {
  const rootResponse = await fetchFrontend('/')
  const rootBody = await rootResponse.text()
  assert.equal(rootResponse.status, 200)
  assert.match(rootResponse.headers.get('content-type') ?? '', /text\/html/)
  assert.match(rootBody, /<div id="app"><\/div>/)

  const deepLinkResponse = await fetchFrontend('/admin/vgpu/monitor/overview')
  const deepLinkBody = await deepLinkResponse.text()
  assert.equal(deepLinkResponse.status, 200)
  assert.match(
    deepLinkResponse.headers.get('content-type') ?? '',
    /text\/html/
  )
  assert.equal(deepLinkBody, rootBody)

  const trailingSlashResponse = await fetchFrontend(
    '/admin/vgpu/monitor/overview/'
  )
  assert.equal(trailingSlashResponse.status, 200)
  assert.equal(await trailingSlashResponse.text(), rootBody)

  const dottedParameterResponse = await fetchFrontend('/redirect/example.com')
  assert.equal(dottedParameterResponse.status, 200)
  assert.equal(await dottedParameterResponse.text(), rootBody)
})

test('keeps the Chart 1.x unrestricted iframe baseline', {
  timeout: 10_000
}, async() => {
  const response = await fetchFrontend('/admin/vgpu/monitor/overview')
  await response.arrayBuffer()

  assert.equal(response.headers.has('x-frame-options'), false)
  assert.doesNotMatch(
    response.headers.get('content-security-policy') ?? '',
    /frame-ancestors/i
  )
})

test('reports Web-entry liveness without asserting backend readiness', {
  timeout: 10_000
}, async() => {
  const response = await fetchFrontend('/health_check')
  const body = await response.text()

  assert.equal(response.status, 200)
  assert.match(body, /OK/)
})

test('reports an unavailable backend as HTTP 502 JSON', {
  timeout: 10_000
}, async() => {
  const response = await fetchFrontend('/api/vgpu/v1/nodes', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: '{}'
  })

  assert.equal(response.status, 502)
  assert.match(response.headers.get('content-type') ?? '', /application\/json/)
  assert.equal(typeof await response.json(), 'object')
})

test('preserves the supported API request and non-success response', {
  timeout: 10_000
}, async() => {
  await listen(backend, backendPort, backendAddress)

  const requestBody = JSON.stringify({
    filters: { name: 'contract-probe' }
  })
  const response = await fetchFrontend('/api/vgpu/v1/nodes?limit=10', {
    method: 'POST',
    headers: {
      'content-type': 'application/json',
      'x-hami-contract-probe': 'preserved'
    },
    body: requestBody
  })
  const responseBody = await response.text()

  assert.deepEqual(lastBackendRequest, {
    method: 'POST',
    url: '/v1/nodes?limit=10',
    contentType: 'application/json',
    probeHeader: 'preserved',
    body: requestBody
  })
  assert.equal(response.status, 400)
  assert.equal(response.headers.get('x-upstream-contract'), 'preserved')
  assert.equal(responseBody, JSON.stringify({ error: 'contract-probe' }))
})

test('reports an upstream timeout as HTTP 504 JSON', {
  timeout: 10_000
}, async() => {
  const response = await fetchFrontend('/api/vgpu/v1/contract-timeout')

  assert.equal(response.status, 504)
  assert.match(response.headers.get('content-type') ?? '', /application\/json/)
  assert.equal(typeof await response.json(), 'object')
})

test('preserves the backend JSON response for an unknown v1 API', {
  timeout: 10_000
}, async() => {
  const response = await fetchFrontend('/api/vgpu/v1/contract-missing')

  assert.equal(response.status, 404)
  assert.match(response.headers.get('content-type') ?? '', /application\/json/)
  assert.deepEqual(await response.json(), {
    error: 'unexpected contract-test request'
  })
})

test('returns 404 for missing static assets and private backend paths', {
  timeout: 10_000
}, async() => {
  const rootBody = await (await fetchFrontend('/')).text()
  for (const requestPath of [
    '/static/contract-missing.js',
    '/metrics',
    '/readyz',
    '/q/openapi.yaml',
    '/api/vgpu/metrics',
    '/api/vgpu/readyz',
    '/api/vgpu/q/openapi.yaml',
    '/api/vgpu/v2/private'
  ]) {
    const response = await fetchFrontend(requestPath)
    const body = await response.text()
    assert.equal(response.status, 404, requestPath)
    assert.equal(response.headers.get('cache-control'), 'no-store', requestPath)
    assert.notEqual(body, rootBody, requestPath)
    assert.notEqual(body, 'private backend response', requestPath)
  }
})

test('serves built assets with bounded caching, gzip and HEAD semantics', {
  timeout: 10_000
}, async() => {
  const indexResponse = await fetchFrontend('/')
  const indexBody = await indexResponse.text()
  assert.match(indexResponse.headers.get('cache-control') ?? '', /no-cache/)
  assert.doesNotMatch(
    indexResponse.headers.get('cache-control') ?? '',
    /immutable/
  )

  const references = [...indexBody.matchAll(
    /(?:src|href)="([^"?#]+\.(?:js|css))"/g
  )].map((match) => match[1])
  const hashedReference = references.find((reference) =>
    /[-.][A-Za-z0-9_-]{8,}\.(?:js|css)$/.test(reference)
  )
  assert.ok(hashedReference, 'built index did not reference a hashed asset')
  const assetPath = new URL(hashedReference, `${frontendURL}/`).pathname

  const identityResponse = await fetchFrontend(assetPath, {
    headers: { 'accept-encoding': 'identity' }
  })
  const identityBody = Buffer.from(await identityResponse.arrayBuffer())
  assert.equal(identityResponse.status, 200)
  assert.match(
    identityResponse.headers.get('cache-control') ?? '',
    /max-age=31536000/
  )
  assert.match(identityResponse.headers.get('cache-control') ?? '', /immutable/)
  assert.equal(identityResponse.headers.has('content-encoding'), false)

  const gzipResponse = await fetchFrontend(assetPath, {
    headers: { 'accept-encoding': 'gzip' }
  })
  const gzipBody = Buffer.from(await gzipResponse.arrayBuffer())
  assert.equal(gzipResponse.status, 200)
  assert.equal(gzipResponse.headers.get('content-encoding'), 'gzip')
  assert.match(gzipResponse.headers.get('vary') ?? '', /Accept-Encoding/i)
  assert.deepEqual(gzipBody, identityBody)

  const headResponse = await fetchFrontend(assetPath, {
    method: 'HEAD',
    headers: { 'accept-encoding': 'gzip' }
  })
  assert.equal(headResponse.status, 200)
  assert.equal(headResponse.headers.get('content-encoding'), 'gzip')
  assert.equal(headResponse.headers.get('cache-control'), gzipResponse.headers.get('cache-control'))
  assert.equal((await headResponse.arrayBuffer()).byteLength, 0)
})

test('exits cleanly on SIGTERM and releases its listener', {
  timeout: 10_000
}, async() => {
  const exited = new Promise((resolve) => frontend.once(
    'exit',
    (code, signal) => resolve({ code, signal })
  ))
  assert.equal(frontend.kill('SIGTERM'), true)
  const result = await withTimeout(
    exited,
    3_000,
    'Web entry did not exit after SIGTERM'
  )
  assert.deepEqual(result, { code: 0, signal: null })

  const probe = http.createServer()
  await listen(probe, frontendPort, backendAddress)
  await closeServer(probe)
})
