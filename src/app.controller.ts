import { Controller, Get, Req, Res } from '@nestjs/common';
import { AppService } from './app.service';
import { Request, Response } from 'express'

@Controller()
export class AppController {
  constructor(private readonly appService: AppService) {}

  @Get('health_check')
  healthCheck(): string {
    return this.appService.healthCheck();
  }

  // api 透传到后api-proxy，health_check, bff 被node接管，其他的直接打回前端vue路由
  @Get('*')
  index(@Req() req: Request, @Res() res: Response) {
    return res.render('index');
  }
}
