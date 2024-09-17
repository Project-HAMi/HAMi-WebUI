const layout = {
  state: {
    sidebarCollapse: false,
  },
  mutations: {
    // 这里填充数据的操作方法
    // 提交方法 $store.commit('changeHello',value)
    changeSidebarCollapse: (state) => {
      state.sidebarCollapse = !state.sidebarCollapse;
    },
    closeSidebar: (state) => {
      state.sidebarCollapse = true;
    },
  },
};

export default layout;
