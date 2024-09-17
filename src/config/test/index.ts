import logger from '../development/logger';
import proxy from '../development/proxy';

// 线上稳定非产线环境（就叫它test环境吧，使用 development 相同的配置）
export default {
  logger: logger,
  proxy: proxy,
};
