// Known deployment paths. Catalog membership never establishes a cluster observation.
const gh = (repo) => 'https://github.com/' + repo;
const guide = (page) => 'https://project-hami.io/docs/userguide/' + page;
const entry = (key, name, publisher, links) => ({ key, name, publisher, ...links });

export const componentCatalog = [
  { vendor: 'NVIDIA', plugins: [
    entry('nvidia-hami', 'HAMi Device Plugin', 'Project-HAMi', { repository: gh('Project-HAMi/HAMi'), docs: 'https://project-hami.io/docs/get-started/deploy-with-helm' }),
  ], exporters: [
    entry('dcgm', 'DCGM Exporter', 'NVIDIA', { repository: gh('NVIDIA/dcgm-exporter'), docs: 'https://project-hami.io/docs/core-concepts/observability' }),
  ] },
  { vendor: 'Ascend', plugins: [
    entry('ascend-hami', 'HAMi Ascend Device Plugin', 'Project-HAMi', { repository: gh('Project-HAMi/ascend-device-plugin'), docs: guide('ascend-device/enable-ascend-sharing') }),
  ], exporters: [
    entry('npu', 'NPU Exporter', 'Ascend MindCluster', { repository: 'https://gitcode.com/Ascend/mind-cluster/tree/master/component/npu-exporter', docs: 'https://www.hiascend.com/document/detail/zh/mindcluster/600/clustersched/introduction/schedulingsd/mxdlug_004.html' }),
  ] },
  { vendor: 'Metax', plugins: [
    entry('metax-native', 'MetaX gpu-device', 'MetaX', { docs: guide('metax-device/metax-gpu/enable-metax-gpu-schedule') }),
  ], exporters: [
    entry('mx', 'mx-exporter', 'MetaX', { repository: gh('MetaX-MACA/mxExporter'), docs: 'https://github.com/MetaX-MACA/mxExporter/blob/main/README.md#deploy-with-kubernetes' }),
  ] },
  { vendor: 'Hygon', plugins: [
    entry('hcu-plugin', 'HCU Device Plugin', 'HYGON-AI', { repository: gh('HYGON-AI/k8s-hcu-device-plugin'), docs: guide('hygon-device/enable-hygon-hcu-sharing') }),
    { ...entry('legacy-dcu', 'DCU vGPU Device Plugin', 'Project-HAMi', { repository: gh('Project-HAMi/dcu-vgpu-device-plugin'), docs: guide('hygon-device/enable-hygon-hcu-sharing') }), archived: true },
  ], exporters: [
    entry('hcu-exporter', 'hcu-exporter', 'HYGON-AI', { repository: gh('HYGON-AI/hcu-exporter'), docs: 'https://github.com/HYGON-AI/hcu-exporter/blob/master/README.md#%E5%BF%AB%E9%80%9F%E5%BC%80%E5%A7%8B' }),
    { ...entry('dcu-exporter', 'dcu-exporter', 'Project-HAMi · DCU', { repository: gh('Project-HAMi/dcu-exporter'), docs: 'https://github.com/Project-HAMi/dcu-exporter/blob/master/README.md#%E5%BF%AB%E9%80%9F%E5%90%AF%E5%8A%A8' }), archived: true },
  ] },
  { vendor: 'MLU', plugins: [
    entry('mlu-plugin', 'Cambricon Device Plugin', 'Cambricon', { repository: gh('Cambricon/cambricon-k8s-device-plugin'), docs: guide('cambricon-device/enable-cambricon-mlu-sharing') }),
  ], exporters: [entry('mlu-exporter', 'MLU Exporter', 'Cambricon', { repository: gh('Cambricon/mlu-exporter'), docs: 'https://github.com/Cambricon/mlu-exporter/blob/master/README.md#installation-and-usage' })] },
  { vendor: 'AMD', componentsOnly: true, plugins: [entry('amd-hami', 'HAMi AMD Device Plugin', 'Project-HAMi', { repository: gh('Project-HAMi/amd-device-plugin'), docs: guide('amd-device/enable-amd-gpu-sharing') })],
    exporters: [entry('amd-metrics', 'AMD Device Metrics Exporter', 'AMD', { repository: gh('ROCm/device-metrics-exporter'), docs: guide('amd-device/configuration#optional-components') })] },
  { vendor: 'Mthreads', componentsOnly: true, plugins: [entry('mt-plugin', 'MT GPU Device Plugin', 'Moore Threads', { docs: guide('mthreads-device/enable-mthreads-gpu-sharing') })],
    exporters: [entry('mt-dcgm', 'DCGM Exporter', 'Moore Threads', { docs: 'https://docs.mthreads.com/cloud-native/cloud-native-doc-online/user_guide/exporter_guide/' })] },
  { vendor: 'Iluvatar', componentsOnly: true, plugins: [entry('ix-manager', 'gpu-manager', 'Iluvatar', { docs: guide('iluvatar-device/enable-iluvatar-gpu-sharing') })],
    exporters: [entry('ix-exporter', 'ix-exporter', 'DeepSpark', { repository: gh('Deep-Spark/ix-exporter'), docs: 'https://github.com/Deep-Spark/ix-exporter#quickstart-on-kubernetes' })] },
  { vendor: 'Enflame', componentsOnly: true, plugins: [entry('gcu-plugin', 'gcushare-device-plugin', 'Enflame', { repository: gh('EnflameTechnology/gcushare'), docs: guide('enflame-device/enable-enflame-gcu-sharing') })],
    exporters: [entry('gcu-exporter', 'gcu-exporter', 'Enflame', { repository: gh('EnflameTechnology/gcu-exporter'), docs: 'https://github.com/EnflameTechnology/gcu-exporter/blob/main/README.md#quick-start' })] },
  { vendor: 'Kunlunxin', componentsOnly: true, plugins: [entry('vxpu', 'vxpu-device-plugin', 'Project-HAMi', { docs: guide('kunlunxin-device/enable-kunlunxin-vxpu') })],
    exporters: [entry('xpu-exporter', 'xpu-exporter', 'Baidu CCE', { docs: 'https://cloud.baidu.com/doc/CProm/s/Bl5uu1n14#xpu-exporter' })] },
  { vendor: 'AWS', componentsOnly: true, plugins: [entry('neuron', 'Neuron Device Plugin', 'AWS Neuron', { repository: gh('aws-neuron/neuron-helm-charts'), docs: guide('awsneuron-device/enable-awsneuron-managing') })],
    exporters: [entry('neuron-monitor', 'Neuron Monitor', 'AWS Neuron', { docs: 'https://awsdocs-neuron.readthedocs-hosted.com/en/latest/tools/neuron-sys-tools/neuron-monitor-user-guide.html' })] },
  { vendor: 'Biren', componentsOnly: true, plugins: [entry('biren-plugin', 'biren-device-plugin', 'Project-HAMi', { repository: gh('Project-HAMi/biren-device-plugin'), docs: guide('biren-device/enable-biren-sharing') })], exporters: [] },
  { vendor: 'Vastai', componentsOnly: true, plugins: [entry('vastai-plugin', 'vastai-device-plugin', 'Project-HAMi', { docs: guide('vastai/enable-vastai-sharing') })],
    exporters: [entry('vastai-exporter', 'Vastai Exporter', 'Vastai')] },
  { vendor: 'Tecorigin', componentsOnly: true, plugins: [], exporters: [] },
];
