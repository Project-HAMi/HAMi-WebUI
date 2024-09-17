const getters = {
  avatar: state => state.user.avatar,
  name: state => state.user.name,
  username: state => state.user.username,
  roles: state => state.user.roles,
  pagePermissions: state => state.user.pagePermission,
  linkPermissions: state => state.user.linkPermissions,
  apiPermissions: state => state.user.apiPermissions,
  detail: state => state.user.detail,
}
export default getters
