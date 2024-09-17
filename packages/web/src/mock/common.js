import Mock from 'mockjs';
import { timeParse } from '@/utils';

const Random = Mock.Random;

Random.extend({
  ip: function () {
    return this.pick(
      Array.from(
        { length: 20 },
        () =>
          `${Math.floor(Math.random() * 256)}.${Math.floor(
            Math.random() * 256,
          )}.${Math.floor(Math.random() * 256)}.${Math.floor(
            Math.random() * 256,
          )}`,
      ),
    );
  },
  status: function () {
    return this.pick(['可用', '不可用']);
  },
  statusPermit: function () {
    return this.pick(['已授权', '未授权', '已过期']);
  },
  time: function () {
    return this.pick(
      Array.from({ length: 20 }, (_, i) => timeParse(Date.now() - i * 10)),
    );
  },
  random: function () {
    return this.pick(
      Array.from({ length: 20 }, () => Math.floor(Math.random() * 256)),
    );
  },
});

// Random.ip();
// Random.status();
// Random.random();

export default Mock;
