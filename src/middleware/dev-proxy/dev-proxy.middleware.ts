import { Injectable, NestMiddleware } from '@nestjs/common';
import { Request, Response, NextFunction } from 'express';

const address = require('address');
const { createProxyMiddleware } = require('http-proxy-middleware');
let localIP = address.ip();

@Injectable()
export class DevProxyMiddleware implements NestMiddleware {
  use(req: Request, res: Response, next: NextFunction) {
    // proxy middleware options
    if(!localIP) {
      localIP = '127.0.0.1'
    }
    /** @type {import('http-proxy-middleware/dist/types').Options} */
    const options = {
      target: `http://${localIP}:8080`,
      changeOrigin: true,
      pathRewrite: (path) => {
        return path;
      },
      router: {},
      secure: false,
      onError: function (err, _req, res) {
        res.writeHead(500, {
          'Content-Type': 'text/plain',
        });
        res.end(
          'Something went wrong. And we are reporting a custom error message.',
        );
      },
      onProxyRes: () => {
      },
      onProxyReq: (proxyReq, req, res) => {
        // 如果没有特殊的Content-Type，则默认为application/json
        const contentType = proxyReq.getHeader('Content-Type') || 'application/json';
        proxyReq.setHeader('Content-Type', contentType);
      },
      logLevel: 'error',
    };

    // 转发所有的静态资源及前端路由，api走api-proxy,health_check和bff层走node本身服务
    const filter = function (pathname) {
      return !/^\/api|health_check|bff\//.test(pathname);
    };
    // create the proxy (with context)
    createProxyMiddleware(filter, options)(req, res, next);
  }
}
