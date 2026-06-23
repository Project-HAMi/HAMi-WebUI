import { Test, TestingModule } from '@nestjs/testing'
import { HttpModule } from '@nestjs/axios'
import { ProxyService } from './proxy.service'

describe('ProxyService', () => {
  let service: ProxyService

  beforeEach(async() => {
    const module: TestingModule = await Test.createTestingModule({
      imports: [HttpModule],
      providers: [ProxyService]
    }).compile()

    service = module.get<ProxyService>(ProxyService)
  })

  it('should be defined', () => {
    expect(service).toBeDefined()
  })
})
