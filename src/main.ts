import { NestFactory } from '@nestjs/core'
import { NestExpressApplication } from '@nestjs/platform-express'
import { AppModule } from './app.module'
import { join } from 'path'
import { TransformInterceptor } from './interceptors/transform.interceptor'
import * as cookieParser from 'cookie-parser';

async function bootstrap() {
  const app = await NestFactory.create<NestExpressApplication>(AppModule, {
    bodyParser: false
  })

  // 指定静态资源目录，这里放的是package下构建的文件
  app.useStaticAssets(join(__dirname, '..', 'public'))
  app.setBaseViewsDir(join(__dirname, '..', 'public'))

  app.setViewEngine('hbs')

  // 注入cookie
  app.use(cookieParser());

  // 统一后端bff层接口返回的格式
  app.useGlobalInterceptors(new TransformInterceptor())

  // 监听3000
  await app.listen(3000)
}

bootstrap()
