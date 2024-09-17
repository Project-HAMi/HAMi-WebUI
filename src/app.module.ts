import {
  Module,
  NestModule,
  MiddlewareConsumer,
  RequestMethod,
} from '@nestjs/common';
import { HttpModule } from '@nestjs/axios';
import * as https from 'https';
import { AppController } from './app.controller';
import { AppService } from './app.service';
import { ApiProxyMiddleware } from './middleware/api-proxy/api-proxy.middleware';
import { DevProxyMiddleware } from './middleware/dev-proxy/dev-proxy.middleware';
import { ProxyService } from './proxy/proxy.service';

@Module({
  imports: [
    HttpModule.registerAsync({
      useFactory: () => ({
        withCredentials: true,
        httpsAgent: new https.Agent({ rejectUnauthorized: false }),
      }),
    }),
  ],
  // 路由按顺序匹配，AppController 用于兜底，故只能放到最最后面
  controllers: [AppController],
  providers: [ AppService,  ProxyService],
})
export class AppModule implements NestModule {
  configure(consumer: MiddlewareConsumer) {
    consumer
      .apply(ApiProxyMiddleware)
      .forRoutes({ path: '/api*', method: RequestMethod.ALL });
      // 开发环境，页面请求走前端代理
    if (process.env.NODE_ENV === 'development') {
      consumer
        .apply(DevProxyMiddleware)
        .forRoutes({ path: '*', method: RequestMethod.GET });
    }
  }
}
