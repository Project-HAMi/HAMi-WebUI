import { Injectable } from '@nestjs/common';
import { HttpService } from '@nestjs/axios';
import { catchError, firstValueFrom } from 'rxjs';
import { AxiosError } from 'axios';
import { AxiosRequestConfig } from 'axios';
import config from '../config';
const proxyConfig = config.proxy;
@Injectable()
export class ProxyService {
  constructor(private readonly httpService: HttpService) {}

  // bff层 请求收拢到统一的位置
  async request(serverName: string, requestConfig: AxiosRequestConfig) {
    const { target } = proxyConfig[serverName];
    const { data } = await firstValueFrom(
      this.httpService
        .request({
          baseURL: target,
          url: requestConfig.url,
          method: requestConfig.method,
          data: requestConfig?.data,
          headers: requestConfig?.headers,
        })
        .pipe(
          catchError((error: AxiosError) => {
            throw new Error('An error happened: ' + requestConfig.url + error);
          }),
        ),
    );
    return data;
  }

  async requestApi(requestConfig: AxiosRequestConfig) {
    const firstSlashIndex = requestConfig.url.indexOf('/');
    const { target } = proxyConfig[requestConfig.url.substring(0, firstSlashIndex)];
    const { data } = await firstValueFrom(
      this.httpService
        .request({
          baseURL: target,
          url: requestConfig.url.slice(firstSlashIndex + 1),
          method: requestConfig.method,
          data: requestConfig?.data,
          headers: requestConfig?.headers,
        })
        .pipe(
          catchError((error: AxiosError) => {
            throw new Error('An error happened: ' + requestConfig.url + error);
          }),
        ),
    );
    return data;
  }
}
