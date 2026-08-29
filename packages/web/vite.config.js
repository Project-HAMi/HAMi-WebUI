import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueJsx from '@vitejs/plugin-vue-jsx'
import { createSvgIconsPlugin } from 'vite-plugin-svg-icons'
import { nodePolyfills } from 'vite-plugin-node-polyfills'
import path from 'path'

const versionedAPIPrefix = '/api/vgpu/v1'
const apiProxyContext = '^/api/vgpu/v1(?:/|[?]|$)'

const hasPathPrefix = (requestPath, prefix) =>
  requestPath === prefix || requestPath.startsWith(`${prefix}/`)

export default defineConfig(() => {
  const backendURL =
    process.env.HAMI_WEBUI_BACKEND_URL || 'http://127.0.0.1:8000'

  return {
    base: './', // 对应 publicPath: './'
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
        // 指定需要缓存的图标文件夹
        iconDirs: [
          path.resolve(process.cwd(), 'src/icons/svg'),
          path.resolve(process.cwd(), 'src/icons/svg/menu') // 确保包含子目录如果需要，或者直接用 src/icons/svg 递归
        ],
        // 指定symbolId格式
        symbolId: 'icon-[name]',
      }),
    ],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
        '~': fileURLToPath(new URL('./projects', import.meta.url)),
        // 兼容 Webpack 的特定 fallback
        // path: 'path-browserify', 这里的 polyfill 由插件处理
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
      outDir: '../../public', // 输出到与 Webpack 相同的目录
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
