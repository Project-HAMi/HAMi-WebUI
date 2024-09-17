const global = {
  state: {
    regionData: {},
  },
  mutations: {
    // 这里填充数据的操作方法
    // 提交方法 $store.commit('changeHello',value)
    changeRegion: (state, regionData) => {
      state.regionData = regionData;
    },
  },
};

export default global;
