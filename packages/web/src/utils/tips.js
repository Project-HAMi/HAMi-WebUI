export const Message = (instance, type, msg, duration) => {
  instance.appContext.config.globalProperties.$Message(type, msg, duration);
}

export const Confirm = (instance, confirmText, confirmObj, fn) => {
  instance.appContext.config.globalProperties.$Confirm(
    confirmText,
    confirmObj,
    fn,
  );
};