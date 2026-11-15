import { buildPhysicalResourceQueries } from '../../../metrics/physical-resource-queries.mjs';

export const getRangeConfigInit = () => {
  const queries = buildPhysicalResourceQueries();
  return [
    {
      key: 'compute',
      dataSource: [
        { key: 'compute-allocation', query: queries.computeAllocation.percentQuery },
        { key: 'compute-usage', query: queries.computeUsage.percentQuery },
      ],
    },
    {
      key: 'memory',
      dataSource: [
        { key: 'memory-allocation', query: queries.memoryAllocation.percentQuery },
        { key: 'memory-usage', query: queries.memoryUsage.percentQuery },
      ],
    },
  ];
};
