import { Injectable, NestMiddleware } from '@nestjs/common';
import { Request, Response, NextFunction } from 'express';
import config from '../../config';

const { createProxyMiddleware } = require('http-proxy-middleware');
const proxyConfig = config.proxy;
const proxyServiceTypes = Object.keys(proxyConfig || {});

@Injectable()
export class ApiProxyMiddleware implements NestMiddleware {

  async use(req: Request, res: Response, next: NextFunction) {
    const filter = function (pathname: string) {
      return proxyServiceTypes.some((item) =>
        pathname.startsWith('/api/' + item),
      );
    };
    const serviceType = proxyServiceTypes.find((item) => {
      return req.path.startsWith('/api/' + item);
    });
    const { target, secure, pathRewrite } = proxyConfig[serviceType] || {};
    // proxy middleware options
    /** @type {import('http-proxy-middleware/dist/types').Options} */
    const options = {
      target: target,
      changeOrigin: true, // needed for virtual hosted sites
      // ws: true, // proxy websockets
      pathRewrite: pathRewrite,
      router: {},
      secure: secure, // UNABLE_TO_VERIFY_LEAF_SIGNATURE
      onError: function (err, _req, res) {
        res.json({
          code: 599,
          reason: 'PROXY_ERROR',
          message: err,
          metadata: {},
        });
      },
      onProxyRes: (proxyRes, req, res) => {},
      onProxyReq: (proxyReq, req, res) => {
        // fy 注意由于使用了express 解析了body，这里发请求的时候需要重新封装成json字符串
        if (req['parsedBody']) {
          const bodyData = JSON.stringify(req['parsedBody']);
          const headers = {
            'Content-Length': Buffer.byteLength(bodyData),
          };
          Object.keys(headers).forEach((key) => {
            proxyReq.setHeader(key, headers[key]);
          });
          proxyReq.write(bodyData);
        }
      },
      logLevel: 'error',
    };

    // create the proxy (with context)
    createProxyMiddleware(filter, options)(req, res, next);
  }
}
