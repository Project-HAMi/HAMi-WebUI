import { NestFactory } from '@nestjs/core'
import { NestExpressApplication } from '@nestjs/platform-express'
import { AppModule } from './app.module'
import { join } from 'path'
import { Request, Response, NextFunction } from 'express'
import { TransformInterceptor } from './interceptors/transform.interceptor'
import * as cookieParser from 'cookie-parser'
import { ENV_BASE_PATH } from './utils/base-path'

async function bootstrap() {
  const app = await NestFactory.create<NestExpressApplication>(AppModule, {
    bodyParser: false
  })

  // Sub-path support (see src/utils/base-path.ts). When the WebUI is served
  // under a non-root prefix by an ingress that passes the full prefixed path
  // through (i.e. does NOT strip it), incoming URLs look like `/gpu-ui/api/...`
  // or `/gpu-ui/static/...`. Strip the configured prefix up-front so that all
  // downstream routing — static assets, the /api* proxy, and the SPA fallback —
  // keeps matching root-relative paths exactly as it does at the site root.
  // A path-stripping proxy (which instead sets X-Forwarded-Prefix) already
  // delivers root-relative URLs, so this middleware is a no-op there.
  if (ENV_BASE_PATH !== '/') {
    const bare = ENV_BASE_PATH.replace(/\/$/, '') // e.g. "/gpu-ui"
    app.use((req: Request, _res: Response, next: NextFunction) => {
      if (req.url === bare) {
        req.url = '/'
      } else if (req.url.startsWith(bare + '/')) {
        req.url = req.url.slice(bare.length) || '/'
      }
      next()
    })
  }

  // 指定静态资源目录，这里放的是package下构建的文件
  // `index: false` disables serve-static's implicit index.html handling so that
  // requests for `/` fall through to AppController, which injects the runtime
  // base path into index.html instead of serving it verbatim. `redirect: false`
  // stops serve-static from 301-redirecting a bare directory request (e.g.
  // `/gpu-ui`, which the prefix-strip middleware above rewrites to `/`) to a
  // trailing-slash URL — the SPA fallback serves it directly instead.
  app.useStaticAssets(join(__dirname, '..', 'public'), {
    index: false,
    redirect: false
  })
  app.setBaseViewsDir(join(__dirname, '..', 'public'))

  app.setViewEngine('hbs')

  // 注入cookie
  app.use(cookieParser())

  // 统一后端bff层接口返回的格式
  app.useGlobalInterceptors(new TransformInterceptor())

  // 监听3000 (overridable via PORT for local testing; container listens on 3000)
  await app.listen(process.env.PORT || 3000)
}

bootstrap()
