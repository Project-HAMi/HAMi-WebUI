const global = {
  state: {
    regionData: {},
  },
  mutations: {
    changeRegion: (state, regionData) => {
      state.regionData = regionData;
    },
  },
};

export default global;
