// TDesign sets a select popup's width once, from offsetWidth when it first opens: fractions round down into
// an ellipsis, and options that widen later, such as counts arriving, are cut. Let the popup follow its
// options instead, never narrower than the trigger or its first width so typing does not make it jump,
// and within TDesign's 1000px.
const MAX_POPUP_WIDTH = 1000;

export const fitPopupToOptions = (trigger, popup) => {
  const opened = Math.max(trigger.offsetWidth, Math.ceil(popup.getBoundingClientRect().width));
  return { width: 'max-content', minWidth: `${Math.min(opened, MAX_POPUP_WIDTH)}px`, maxWidth: `${MAX_POPUP_WIDTH}px` };
};
