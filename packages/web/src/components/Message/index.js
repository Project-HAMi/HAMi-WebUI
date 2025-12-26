import MessageComponent from './index.vue'
import { createVNode, render } from 'vue'
import i18n from '@/locales'

const Message = {
    install(app) {
      const Vnode = createVNode(MessageComponent)
  		render(Vnode, document.body)
      app.config.globalProperties.$Message = (type, msg, duration) => {
        if(!msg){
            msg = type==='success'?i18n.global.t('common.operationSuccess'):type==='info'?i18n.global.t('common.messageTip'):type==='warning'?i18n.global.t('common.operationWarning'):type==='danger'?i18n.global.t('common.operationFailed'):''
        }
        duration = duration || 2000
				Vnode.component.ctx.showMessage(type, msg, duration)
				setTimeout(()=>{
					Vnode.component.ctx.hideMessage()
				}, duration)
      };
    }
  };

export default Message

