import type { InjectionKey, MaybeRefOrGetter } from 'vue';
import { ref } from 'vue';
import { useToast } from './toast';

// copiedLabelKey lets the app give every CopyButton its translated Copied toast.
export const copiedLabelKey: InjectionKey<MaybeRefOrGetter<string>> = Symbol('copiedLabel');

// useCopy writes a value to the clipboard, flags it as copied for a moment, and announces copiedLabel as a toast when given.
export function useCopy() {
  const toast = useToast();
  const copied = ref(false);

  async function copy(value: string, copiedLabel = '') {
    try {
      await navigator.clipboard.writeText(value);
      copied.value = true;
      if (copiedLabel) {
        toast.push('success', copiedLabel);
      }
      setTimeout(() => {
        copied.value = false;
      }, 1200);
    } catch {
      // clipboard unavailable
    }
  }

  return { copied, copy };
}
