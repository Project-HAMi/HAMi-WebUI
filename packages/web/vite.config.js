import { fileURLToPath, URL } from 'node:url'
import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueJsx from '@vitejs/plugin-vue-jsx'
import { createSvgIconsPlugin } from 'vite-plugin-svg-icons'
import { nodePolyfills } from 'vite-plugin-node-polyfills'
import path from 'path'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd())
  
  return {
    base: './', // 对应 publicPath: './'
    plugins: [
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
      port: 8080,
      proxy: {
        '/api': {
          target: 'http://127.0.0.1:3000',
          changeOrigin: true,
        },
        '/ws': {
            target: 'ws://127.0.0.1:3000',
            ws: true
        }
      }
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
    define: {
      // 注入 Webpack 环境变量
      'process.env': { ...process.env, ...env }
    }
  }
})
