import { Test, TestingModule } from '@nestjs/testing';
import { ProxyService } from './proxy.service';
import { HttpService } from '@nestjs/axios';
import { of } from 'rxjs';

describe('ProxyService', () => {
  let service: ProxyService;
  let httpService: HttpService;

  const mockHttpService = {
    request: jest.fn(),
  };

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      providers: [
        ProxyService,
        {
          provide: HttpService,
          useValue: mockHttpService,
        },
      ],
    }).compile();

    service = module.get<ProxyService>(ProxyService);
    httpService = module.get<HttpService>(HttpService);
  });

  it('should be defined', () => {
    expect(service).toBeDefined();
  });
});
