import path from 'path'
/**
 * 获取所有子路由
 */
const getChildrenRoutes = routes => {
  const result = []
  routes.forEach(route => {
    if(route.children && route.children.length > 0) {
      result.push(...route.children)
    }
  })
  return result
}

/**
 * 删除脱离层级的子路由
 */
export const filterRoutes = routes => {
  const childrenRoutes = getChildrenRoutes(routes)
  return routes.filter(route => {
    return !childrenRoutes.find(childrenRoute => {
      return childrenRoute.path === route.path
    })
  })
}

const isNull = (data) => {
  if(!data) return true
  if(JSON.stringify(data) === '{}') return true
  if(JSON.stringify(data) === '[]') return true
  return false
}

export const generatorMenus = (routes, basePath = '') => {
  const result = []
  routes.forEach(item => {
    if(isNull(item.meta) && isNull(item.children)) return
    if(isNull(item.meta) && !isNull(item.children)) {
      result.push(...generatorMenus(item.children))
      return
    }
    const routePath = path.resolve(basePath, item.path)
    let route = result.find(item => item.path === routePath)
    if(!route){
      route = {
        ...item,
        path: routePath,
        children: []
      }
      if(route.name){
        result.push(route)
      }
    }
    if(item.children) {
      route.children.push(...generatorMenus(item.children, route.path))
    }
  })
  return result
}