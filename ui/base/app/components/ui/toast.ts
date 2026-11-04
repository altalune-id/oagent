import { reactive } from 'vue';

export type ToastKind = 'success' | 'info' | 'error';

export interface Toast {
  id: number;
  kind: ToastKind;
  text: string;
}

const toasts = reactive<Toast[]>([]);
const timers = new Map<number, ReturnType<typeof setTimeout>>();
let next = 1;

export const toastLifetimeMs = 5000;

function dismiss(id: number) {
  clearTimeout(timers.get(id));
  timers.delete(id);
  const at = toasts.findIndex(t => t.id === id);
  if (at >= 0) {
    toasts.splice(at, 1);
  }
}

function schedule(id: number) {
  clearTimeout(timers.get(id));
  timers.set(id, setTimeout(dismiss, toastLifetimeMs, id));
}

function pause(id: number) {
  clearTimeout(timers.get(id));
  timers.delete(id);
}

function push(kind: ToastKind, text: string): number {
  const id = next++;
  toasts.push({ id, kind, text });
  schedule(id);
  return id;
}

// useToast is the app-wide toast queue; ToastHost renders it.
export function useToast() {
  return { toasts, push, dismiss, pause, resume: schedule };
}
