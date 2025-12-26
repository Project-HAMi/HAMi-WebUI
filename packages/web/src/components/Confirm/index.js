import ConfirmComponent from './index.vue'
import { createVNode, render } from 'vue'
import i18n from '@/locales'

const Confirm = {
    install(app) {
        const confirmVnode = createVNode(ConfirmComponent)
        render(confirmVnode, document.body)
        app.config.globalProperties.$Confirm = (confirmText, confirmObj, fn) => {
            if(JSON.stringify(confirmObj) === '{}') {
                confirmObj = {
                    confirmButtonText: i18n.global.t('common.confirm'),
                    cancelButtonText: i18n.global.t('common.cancel'),
                    type: '',
                }
            }
            confirmVnode.component.ctx.showConfirm(confirmText, confirmObj, fn)
        };
      }
}

export default Confirm

