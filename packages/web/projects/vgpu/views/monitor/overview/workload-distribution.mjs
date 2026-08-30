const normalizeWorkloadCount = (value) => {
  const number = Number(value);
  return Number.isFinite(number) && number >= 0 ? number : 0;
};

export const createWorkloadDistributionOptions = ({
  rows = [],
  translate,
  bucketSize = 10,
}) => {
  if (!Number.isFinite(bucketSize) || bucketSize <= 0) {
    throw new TypeError('bucketSize must be a positive number');
  }

  const values = (Array.isArray(rows) ? rows : [])
    .map((item) => normalizeWorkloadCount(item?.value));
  const maxValue = values.length ? Math.max(...values) : 0;
  const bucketCount = Math.max(1, Math.floor(maxValue / bucketSize) + 1);
  const categories = Array.from({ length: bucketCount }, (_, index) => {
    const start = index * bucketSize;
    return `${start}-${start + bucketSize - 1}`;
  });
  const counts = new Array(bucketCount).fill(0);

  values.forEach((value) => {
    const bucketIndex = Math.min(
      Math.floor(value / bucketSize),
      bucketCount - 1,
    );
    counts[bucketIndex] += 1;
  });

  return {
    tooltip: {
      trigger: 'item',
      formatter: (item) => {
        if (!item) return '';
        const unit = translate('common.unitCount');
        return `${translate('dashboard.workloadRange')}: ${item.name}<br/>${translate('dashboard.nodeTotal')}: ${Number(item.value || 0)}${unit}`;
      },
    },
    grid: {
      left: 25,
      right: 25,
      top: 20,
      bottom: 8,
      outerBoundsMode: 'same',
      outerBoundsContain: 'axisLabel',
    },
    xAxis: {
      type: 'category',
      data: categories,
      axisTick: {
        alignWithLabel: true,
      },
      axisLabel: {
        color: '#697886',
      },
    },
    dataZoom: [
      {
        type: 'inside',
        xAxisIndex: 0,
        filterMode: 'none',
      },
    ],
    yAxis: {
      type: 'value',
      minInterval: 1,
      axisLine: {
        show: false,
      },
      axisTick: {
        show: false,
      },
      axisLabel: {
        color: '#697886',
      },
    },
    series: [
      {
        data: counts,
        type: 'bar',
        barWidth: 24,
        barCategoryGap: '75',
        itemStyle: {
          color: '#5B8FF9',
        },
      },
    ],
  };
};
