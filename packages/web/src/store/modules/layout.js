const layout = {
  state: {
    sidebarCollapse: false,
  },
  mutations: {
    changeSidebarCollapse: (state) => {
      state.sidebarCollapse = !state.sidebarCollapse;
    },
    closeSidebar: (state) => {
      state.sidebarCollapse = true;
    },
  },
};

export default layout;
