// focusWhenShown focuses el, retrying on each animation frame until the browser has rendered it focusable, for at most `frames` frames.
export function focusWhenShown(el: HTMLElement, frames = 10): void {
  el.focus();
  if (document.activeElement !== el && frames > 0) {
    requestAnimationFrame(() => focusWhenShown(el, frames - 1));
  }
}

// focusIfLost focuses el when focus has fallen back to the page body or sits on a control that became disabled.
export function focusIfLost(el: HTMLElement | null | undefined): void {
  const active = document.activeElement;
  if (el && (!active || active === document.body || (active as HTMLInputElement).disabled === true)) {
    focusWhenShown(el);
  }
}
