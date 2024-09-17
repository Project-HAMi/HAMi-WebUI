import MessageComponent from './index.vue'
import { createVNode, render } from 'vue'
const Message = {
    install(app) {
      const Vnode = createVNode(MessageComponent)
  		render(Vnode, document.body)
      app.config.globalProperties.$Message = (type, msg, duration) => {
        if(!msg){
            msg = type==='success'?'操作成功':type==='info'?'消息提示':type==='warning'?'操作警告':type==='danger'?'操作失败':''
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

