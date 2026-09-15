const positive = (value) => {
  const number = Number(value);
  return Number.isFinite(number) && number > 0 ? number : undefined;
};

export const DEVICE_CONFIG_STATES = ['disabled', 'loading', 'missing', 'forbidden', 'invalid', 'error'];

// int64 fields arrive as strings; absent AI Core counts stay undefined.
export const findAscendModel = (config, type) => {
  if (config?.state !== 'loaded' || !type) return undefined;
  const model = (Array.isArray(config.ascendModels) ? config.ascendModels : [])
    .find((item) => item?.commonWord === type);
  if (!model) return undefined;
  return {
    commonWord: model.commonWord,
    chipName: model.chipName || '',
    aiCore: positive(model.aiCore),
    aiCpu: positive(model.aiCpu),
    memoryAllocatableMiB: positive(model.memoryAllocatable),
    superPod: model.superPod === true,
    templates: (Array.isArray(model.templates) ? model.templates : []).map((template) => ({
      name: template.name,
      memoryMiB: positive(template.memory),
      aiCore: positive(template.aiCore),
      aiCpu: positive(template.aiCpu),
      computeShare: positive(template.computeShare),
    })),
  };
};

const share = (used, total) => ({
  used,
  total,
  percent: used !== undefined && total ? Math.min(100, Math.round((used / total) * 100)) : undefined,
});

// Each template as a share of one card, followed by the whole card.
export const buildAllocationOptions = (model) => {
  if (!model) return [];
  const option = (name, memoryMiB, aiCore, aiCpu, computeShare, whole = false) => ({
    name,
    whole,
    computeShare,
    aiCore: share(aiCore, model.aiCore),
    aiCpu: share(aiCpu, model.aiCpu),
    memory: share(memoryMiB, model.memoryAllocatableMiB),
  });
  return [
    ...model.templates.map((template) => option(template.name, template.memoryMiB, template.aiCore, template.aiCpu, template.computeShare)),
    option('', model.memoryAllocatableMiB, model.aiCore, model.aiCpu, 100, true),
  ];
};

export const getDeviceConfigStateKey = (config) => {
  const state = config?.state;
  if (state === 'loaded') return '';
  return `card.deviceConfig.state.${DEVICE_CONFIG_STATES.includes(state) ? state : 'error'}`;
};
