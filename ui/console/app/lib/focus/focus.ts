// focusWhenShown focuses el, retrying on each animation frame until the browser has rendered it focusable, for at most `frames` frames.
export function focusWhenShown(el: HTMLElement, frames = 10): void {
  el.focus();
  if (document.activeElement !== el && frames > 0) {
    requestAnimationFrame(() => focusWhenShown(el, frames - 1));
  }
}
