import {
  Injectable,
  NestInterceptor,
  ExecutionContext,
  CallHandler,
} from '@nestjs/common';
import { Observable } from 'rxjs';
import { map } from 'rxjs/operators';

type CustomResponse<T> = {
  code: number;
  msg: string;
  data: T;
};

@Injectable()
export class TransformInterceptor<T>
  implements NestInterceptor<T, CustomResponse<T>>
{
  intercept(
    context: ExecutionContext,
    next: CallHandler,
  ): Observable<CustomResponse<T>> {
    return next.handle().pipe(
      map((data) => {
        return {
          code: 0,
          msg: 'success',
          data,
        };
      }),
    );
  }
}
