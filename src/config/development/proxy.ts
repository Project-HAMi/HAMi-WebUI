export default {
  vgpu: {
    target: 'http://192.168.10.137:8000',
    secure: false,
    pathRewrite: {
      '^/api/vgpu': '',
    },
  },
};
