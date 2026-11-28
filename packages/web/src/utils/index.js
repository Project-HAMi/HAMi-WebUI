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
    throw new TypeError('num and decimalPlaces must be numbers');
  }

  if (num % 1 !== 0) {
    const factor = Math.pow(10, decimalPlaces);
    return Math.round(num * factor) / factor;
  }

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
