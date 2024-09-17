import ConfirmComponent from './index.vue'
import { createVNode, render } from 'vue'

const Confirm = {
    install(app) {
        const confirmVnode = createVNode(ConfirmComponent)
        render(confirmVnode, document.body)
        app.config.globalProperties.$Confirm = (confirmText, confirmObj, fn) => {
            if(JSON.stringify(confirmObj) === '{}') {
                confirmObj = {
                    confirmButtonText: '确定',
                    cancelButtonText: '取消',
                    type: '',
                }
            }
            confirmVnode.component.ctx.showConfirm(confirmText, confirmObj, fn)
        };
      }
}

export default Confirm

