import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueJsx from '@vitejs/plugin-vue-jsx'
import { createSvgIconsPlugin } from 'vite-plugin-svg-icons'
import { nodePolyfills } from 'vite-plugin-node-polyfills'
import path from 'path'

// vite-plugin-svg-icons 2.0.1 imports fast-glob without declaring it. Keep the
// explicit Web devDependency until the plugin fixes its package metadata.

const versionedAPIPrefix = '/api/vgpu/v1'
const apiProxyContext = '^/api/vgpu/v1(?:/|[?]|$)'

const hasPathPrefix = (requestPath, prefix) =>
  requestPath === prefix || requestPath.startsWith(`${prefix}/`)

export default defineConfig(() => {
  const backendURL =
    process.env.HAMI_WEBUI_BACKEND_URL || 'http://127.0.0.1:8000'

  return {
    // Keep assets relative so the Web entry can inject the runtime base path.
    base: './',
    plugins: [
      {
        name: 'hami-webui-dev-api-boundary',
        configureServer(server) {
          server.middlewares.use((request, response, next) => {
            const requestPath = new URL(
              request.url || '/',
              'http://vite.local'
            ).pathname
            const isAPIRequest = hasPathPrefix(requestPath, '/api')

            if (isAPIRequest && !hasPathPrefix(requestPath, versionedAPIPrefix)) {
              response.statusCode = 404
              response.setHeader('Cache-Control', 'no-store')
              response.setHeader('Content-Type', 'text/plain; charset=utf-8')
              response.end('404 page not found\n')
              return
            }

            next()
          })
        },
      },
      vue(),
      vueJsx(),
      nodePolyfills({
        include: ['crypto', 'stream', 'util', 'process', 'buffer'],
        globals: {
          Buffer: true,
          global: true,
          process: true,
        },
      }),
      createSvgIconsPlugin({
        // The parent directory is scanned recursively; listing menu separately
        // would emit duplicate symbol IDs for every menu icon.
        iconDirs: [path.resolve(process.cwd(), 'src/icons/svg')],
        symbolId: 'icon-[name]',
      }),
    ],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
        '~': fileURLToPath(new URL('./projects', import.meta.url)),
      },
      extensions: ['.mjs', '.js', '.ts', '.jsx', '.tsx', '.json', '.vue']
    },
    server: {
      port: 3000,
      strictPort: true,
      proxy: {
        [apiProxyContext]: {
          target: backendURL,
          changeOrigin: true,
          rewrite: (requestPath) =>
            requestPath.replace(/^\/api\/vgpu(?=\/|\?|$)/, ''),
        },
      },
    },
    build: {
      // The Go Web entry serves this generated directory.
      outDir: '../../public',
      assetsDir: 'static',
      emptyOutDir: true,
    },
    css: {
      preprocessorOptions: {
        scss: {
          api: 'modern-compiler',
          silenceDeprecations: ['legacy-js-api', 'import'],
        },
      },
    },
  }
})
