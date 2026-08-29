import assert from 'node:assert/strict'
import http from 'node:http'
import { after, before, test } from 'node:test'
import { setTimeout as delay } from 'node:timers/promises'
import { launchWebEntry } from './launch-web-entry.mjs'

const backendAddress = '127.0.0.1'
const backendPort = 8000
const frontendURL = 'http://127.0.0.1:3000'

let backend
let frontend
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

async function assertPortAvailable(port, host) {
  const probe = http.createServer()
  try {
    await listen(probe, port, host)
  } catch (error) {
    if (error.code === 'EADDRINUSE') {
      throw new Error(
        `Contract test requires ${host ?? 'all interfaces'}:${port} ` +
        'to be unused; ' +
        'refusing to test against an unrelated process'
      )
    }
    throw error
  }
  await closeServer(probe)
}

async function stopProcess(child) {
  if (!child || child.exitCode !== null) return

  const exited = new Promise((resolve) => child.once('exit', resolve))
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
    if (frontend.exitCode !== null) {
      throw new Error(
        `Web entry exited with code ${frontend.exitCode}.\n${frontendLogs}`
      )
    }

    try {
      const response = await fetch(`${frontendURL}/health_check`, {
        signal: AbortSignal.timeout(500)
      })
      await response.arrayBuffer()
      if (response.ok) {
        await delay(25)
        if (frontend.exitCode === null) return
      }
    } catch {
      // The process may still be binding the port.
    }
    await delay(100)
  }

  throw new Error(`Timed out waiting for the Web entry.\n${frontendLogs}`)
}

before(async() => {
  await assertPortAvailable(3000)
  await assertPortAvailable(backendPort, backendAddress)

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

    if (req.method !== 'POST' || req.url !== '/v1/nodes') {
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

  frontend = launchWebEntry(process.cwd())
  captureFrontendLogs(frontend.stdout)
  captureFrontendLogs(frontend.stderr)
  await waitForFrontend()
  await listen(backend, backendPort, backendAddress)
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

test('preserves the supported API request and non-success response', {
  timeout: 10_000
}, async() => {
  const requestBody = JSON.stringify({
    filters: { name: 'contract-probe' }
  })
  const response = await fetchFrontend('/api/vgpu/v1/nodes', {
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
    url: '/v1/nodes',
    contentType: 'application/json',
    probeHeader: 'preserved',
    body: requestBody
  })
  assert.equal(response.status, 400)
  assert.equal(response.headers.get('x-upstream-contract'), 'preserved')
  assert.equal(responseBody, JSON.stringify({ error: 'contract-probe' }))
})
