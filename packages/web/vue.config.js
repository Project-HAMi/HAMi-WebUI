const { defineConfig } = require('@vue/cli-service');
const path = require('path');
function resolve(dir) {
  return path.join(__dirname, dir);
}
const defaultSettings = require('./src/settings.js');
const name = defaultSettings.title || 'vue Element Admin'; // page title

const NodePolyfillPlugin = require('node-polyfill-webpack-plugin');

const port = process.env.port || process.env.npm_config_port || 8080; // dev port

module.exports = defineConfig({
  transpileDependencies: true,
  publicPath: '/',
  outputDir: path.join(__dirname, '../../', 'public'),
  indexPath: 'index.hbs',
  assetsDir: 'static',
  lintOnSave: process.env.NODE_ENV === 'development',
  productionSourceMap: false,
  configureWebpack: {
    // provide the app's title in webpack's name field, so that
    // it can be accessed in index.html to inject the correct title.
    name: name,
    resolve: {
      alias: {
        '@': resolve('src'),
        '~': resolve('projects'),
      },
      fallback: {
        fs: false,
        crypto: require.resolve('crypto-browserify'),
      },
    },
    plugins: [new NodePolyfillPlugin()],
    cache: {
      type: 'filesystem',
      // 可选配置
      buildDependencies: {
        config: [__filename],
      },
    },
  },
  chainWebpack(config) {
    // 设置 svg-sprite-loader
    config.module.rule('svg').exclude.add(resolve('src/icons')).end();
    config.module
      .rule('icons')
      .test(/\.svg$/)
      .include.add(resolve('src/icons'))
      .end()
      .use('svg-sprite-loader')
      .loader('svg-sprite-loader')
      .options({
        symbolId: 'icon-[name]',
      })
      .end();
  },
  devServer: {
    port: port,
    client: {
      overlay: false,
      logging: 'info',
      webSocketURL: `ws://0.0.0.0:${port}/ws`,
    },
  },
});
