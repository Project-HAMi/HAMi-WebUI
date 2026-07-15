import { Controller, Get, Req, Res } from '@nestjs/common'
import { AppService } from './app.service'
import { Request, Response } from 'express'
import { join } from 'path'
import { readFileSync } from 'fs'
import { injectBasePath, resolveBasePath } from './utils/base-path'

@Controller()
export class AppController {
  constructor(private readonly appService: AppService) {}

  // index.html is read from disk once and cached; the per-request base path is
  // injected on every send, so the same built artifact serves any sub-path.
  private indexTemplate: string | null = null

  private readonly indexPath = join(__dirname, '..', 'public', 'index.html')

  @Get('health_check')
  healthCheck(): string {
    return this.appService.healthCheck()
  }

  // api 透传到后api-proxy，health_check, bff 被node接管，其他的直接打回前端vue路由
  @Get('*')
  index(@Req() req: Request, @Res() res: Response) {
    const basePath = resolveBasePath(req)
    res.type('html').send(this.renderIndex(basePath))
  }

  // Return index.html with the resolved base path injected at request time
  // (see injectBasePath). The template is read from disk once and cached.
  private renderIndex(basePath: string): string {
    if (this.indexTemplate === null) {
      this.indexTemplate = readFileSync(this.indexPath, 'utf-8')
    }
    return injectBasePath(this.indexTemplate, basePath)
  }
}
