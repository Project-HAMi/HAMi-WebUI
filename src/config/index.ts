/**
 * 判断当前指示当前环境的常量返回对应配置
 * 默认返回开发环境的配置
 */
const nodeEnv = process.env.NODE_ENV;
const configName = ['development', 'production', 'test'].includes(nodeEnv)
  ? nodeEnv
  : 'production';
// eslint-disable-next-line @typescript-eslint/no-var-requires
const config = require(`./${configName}`);

export default config.default;
