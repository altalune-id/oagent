// focusWhenShown focuses target once container is no longer visibility hidden, as a daisyUI drawer fades in; it gives up waiting after about half a second.
export function focusWhenShown(container: () => HTMLElement | undefined, target: () => HTMLElement | null | undefined, active: () => boolean, frames = 30) {
  if (!active()) {
    return;
  }
  const box = container();
  if (frames === 0 || !box || getComputedStyle(box).visibility !== 'hidden') {
    target()?.focus();
    return;
  }
  requestAnimationFrame(() => focusWhenShown(container, target, active, frames - 1));
}
