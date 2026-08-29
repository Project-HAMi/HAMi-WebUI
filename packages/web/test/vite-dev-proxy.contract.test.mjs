import assert from 'node:assert/strict'
import { once } from 'node:events'
import {
  createServer as createHTTPServer,
  request as createHTTPRequest,
} from 'node:http'
import { test } from 'node:test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { createServer as createViteServer, loadConfigFromFile } from 'vite'

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const configFile = path.join(webRoot, 'vite.config.js')

const listen = async(server) => {
  server.listen(0, '127.0.0.1')
  await once(server, 'listening')

  const address = server.address()
  assert.notEqual(address, null)
  assert.equal(typeof address, 'object')
  return `http://127.0.0.1:${address.port}`
}

const close = async(server) => {
  if (!server.listening) return
  server.close()
  await once(server, 'close')
}

const getRawPath = async(origin, requestPath) => {
  const url = new URL(origin)

  return new Promise((resolve, reject) => {
    const request = createHTTPRequest(
      {
        hostname: url.hostname,
        method: 'GET',
        path: requestPath,
        port: url.port,
      },
      async(response) => {
        const chunks = []
        for await (const chunk of response) chunks.push(chunk)
        resolve({
          body: Buffer.concat(chunks).toString('utf8'),
          headers: response.headers,
          status: response.statusCode,
        })
      }
    )
    request.on('error', reject)
    request.end()
  })
}

test('Vite serves the SPA and proxies the versioned Web API without NestJS', async() => {
  const requests = []
  const backend = createHTTPServer(async(request, response) => {
    const chunks = []
    for await (const chunk of request) chunks.push(chunk)

    requests.push({
      body: Buffer.concat(chunks).toString('utf8'),
      headers: request.headers,
      method: request.method,
      url: request.url,
    })

    response.writeHead(422, {
      'content-type': 'application/json',
      'x-contract-response': 'preserved',
    })
    response.end(JSON.stringify({ code: 422, message: 'contract response' }))
  })

  const previousBackendURL = process.env.HAMI_WEBUI_BACKEND_URL
  let frontend
  let vite

  try {
    delete process.env.HAMI_WEBUI_BACKEND_URL
    const loadedConfig = await loadConfigFromFile(
      { command: 'serve', mode: 'development' },
      configFile,
      webRoot,
      'silent'
    )
    assert.notEqual(loadedConfig, null)
    assert.equal(
      loadedConfig.config.server.proxy['^/api/vgpu/v1(?:/|[?]|$)'].target,
      'http://127.0.0.1:8000'
    )

    const backendURL = await listen(backend)
    process.env.HAMI_WEBUI_BACKEND_URL = backendURL

    vite = await createViteServer({
      configFile,
      logLevel: 'silent',
      optimizeDeps: {
        include: [],
        noDiscovery: true,
      },
      root: webRoot,
      server: {
        middlewareMode: true,
      },
    })

    assert.equal(vite.config.server.port, 3000)
    assert.equal(vite.config.server.strictPort, true)
    assert.deepEqual(Object.keys(vite.config.server.proxy), [
      '^/api/vgpu/v1(?:/|[?]|$)',
    ])
    assert.equal(
      vite.config.server.proxy['^/api/vgpu/v1(?:/|[?]|$)'].target,
      backendURL
    )

    frontend = createHTTPServer(vite.middlewares)
    const frontendURL = await listen(frontend)

    const apiResponse = await fetch(
      `${frontendURL}/api/vgpu/v1/nodes?limit=10&name=contract%20probe`,
      {
        body: JSON.stringify({ node: 'worker-1' }),
        headers: {
          'content-type': 'application/json',
          'x-contract-request': 'preserved',
        },
        method: 'POST',
      }
    )

    assert.equal(apiResponse.status, 422)
    assert.equal(apiResponse.headers.get('x-contract-response'), 'preserved')
    assert.deepEqual(await apiResponse.json(), {
      code: 422,
      message: 'contract response',
    })
    assert.equal(requests.length, 1)
    assert.equal(requests[0].method, 'POST')
    assert.equal(
      requests[0].url,
      '/v1/nodes?limit=10&name=contract%20probe'
    )
    assert.equal(requests[0].body, JSON.stringify({ node: 'worker-1' }))
    assert.equal(requests[0].headers['content-type'], 'application/json')
    assert.equal(requests[0].headers['x-contract-request'], 'preserved')

    const deepLinkResponse = await fetch(
      `${frontendURL}/admin/vgpu/monitor/overview`,
      { headers: { accept: 'text/html' } }
    )
    assert.equal(deepLinkResponse.status, 200)
    assert.match(await deepLinkResponse.text(), /<div id="app"><\/div>/)

    for (const blockedPath of [
      '/api/unknown',
      '/api/vgpu-extra/v1/nodes',
      '/api/vgpu/metrics',
      '/api/vgpu/readyz',
      '/api/vgpu/q/openapi.yaml',
      '/api/vgpu/v2/private',
      '/api/vgpu/v1/../metrics',
    ]) {
      const blockedResponse = await getRawPath(frontendURL, blockedPath)
      assert.equal(blockedResponse.status, 404, blockedPath)
      assert.equal(
        blockedResponse.headers['cache-control'],
        'no-store',
        blockedPath
      )
      assert.equal(blockedResponse.body, '404 page not found\n')
      assert.equal(requests.length, 1, blockedPath)
    }
  } finally {
    if (frontend) await close(frontend)
    if (vite) await vite.close()
    await close(backend)

    if (previousBackendURL === undefined) {
      delete process.env.HAMI_WEBUI_BACKEND_URL
    } else {
      process.env.HAMI_WEBUI_BACKEND_URL = previousBackendURL
    }
  }
})
