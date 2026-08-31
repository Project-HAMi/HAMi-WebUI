export const timeParse = (obj = new Date(), format = 'YYYY-MM-DD HH:mm:ss') => {
  obj = new Date(obj);
  const year = obj.getFullYear();
  const mon =
    obj.getMonth() + 1 < 10 ? '0' + (obj.getMonth() + 1) : obj.getMonth() + 1;
  const day = obj.getDate() < 10 ? '0' + obj.getDate() : obj.getDate();
  const hour = obj.getHours() < 10 ? '0' + obj.getHours() : obj.getHours();
  const min = obj.getMinutes() < 10 ? '0' + obj.getMinutes() : obj.getMinutes();
  const second =
    obj.getSeconds() < 10 ? '0' + obj.getSeconds() : obj.getSeconds();

  format = format
    .replace('YYYY', year + '')
    .replace('MM', mon + '')
    .replace('DD', day + '')
    .replace('HH', hour + '')
    .replace('mm', min + '')
    .replace('ss', second + '');

  return format;
};

export function roundToDecimal(num, decimalPlaces) {
  if (typeof num !== 'number' || typeof decimalPlaces !== 'number') {
    throw new TypeError('参数必须是数字');
  }

  // 判断是否是小数
  if (num % 1 !== 0) {
    // 进行四舍五入
    const factor = Math.pow(10, decimalPlaces);
    return Math.round(num * factor) / factor;
  }

  // 如果不是小数，返回原数字
  return num;
}

export const getResourceColor = (percentage) => {
  if (percentage >= 80) {
    return '#DC2626';
  } else if (percentage >= 50) {
    return '#2563EB';
  } else {
    return '#16A34A';
  }
};

/**
 * Calculate Prometheus query step based on time range to avoid exceeding 11,000 data points limit
 */
export const calculatePrometheusStep = (startTime, endTime, maxPoints = 11000) => {
  const start = new Date(startTime);
  const end = new Date(endTime);

  if (isNaN(start.getTime()) || isNaN(end.getTime())) {
    return '1m';
  }

  const durationMs = end.getTime() - start.getTime();

  if (durationMs <= 0 || !isFinite(durationMs)) {
    return '1m';
  }

  const durationMinutes = durationMs / (1000 * 60);
  const minStepMinutes = Math.ceil(durationMinutes / maxPoints);

  if (durationMinutes <= 15) {
    // ≤ 15 minutes: second-level precision
    const points15s = Math.ceil(durationMinutes / 0.25);
    const points30s = Math.ceil(durationMinutes / 0.5);
    if (points15s <= maxPoints) return '15s';
    if (points30s <= maxPoints) return '30s';
    return '1m';
  } else if (durationMinutes <= 60) {
    return minStepMinutes <= 1 ? '1m' : '5m';
  } else if (durationMinutes <= 360) {
    return minStepMinutes <= 5 ? '5m' : '15m';
  } else if (durationMinutes <= 1440) {
    return minStepMinutes <= 15 ? '15m' : '30m';
  } else if (durationMinutes <= 10080) {
    return minStepMinutes <= 30 ? '30m' : '1h';
  } else if (durationMinutes <= 43200) {
    return minStepMinutes <= 60 ? '1h' : '6h';
  } else {
    const durationHours = durationMinutes / 60;
    const dataPointsWith24h = Math.ceil(durationHours / 24);
    
    if (dataPointsWith24h <= maxPoints) {
      return '24h';
    }

    const minStepDays = Math.ceil(dataPointsWith24h / maxPoints);

    if (!isFinite(minStepDays) || isNaN(minStepDays) || minStepDays <= 0) {
      return '24h';
    }

    return `${minStepDays * 24}h`;
  }
};
