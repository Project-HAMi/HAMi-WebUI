import ElementPlus from 'element-plus'
import locale from 'element-plus/dist/locale/zh-cn.mjs'

export default (app) => {
  app.use(ElementPlus, { locale })
}
