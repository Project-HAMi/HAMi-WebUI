import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import locale from 'element-plus/dist/locale/zh-cn.mjs'

export default (app) => {
  app.use(ElementPlus, { locale })
}