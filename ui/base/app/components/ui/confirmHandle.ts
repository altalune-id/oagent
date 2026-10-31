import type { Ref } from 'vue';
import type { ShownError } from '../../lib/apperror/message';
import type { ConfirmHandle } from './types';
import { ref } from 'vue';

// confirmHandle runs one destructive action on a target after a confirm, keeping the confirm open with toError's message on a refusal.
export function confirmHandle<T>(run: (target: T) => Promise<void>, toError: (err: unknown) => ShownError) {
  const target = ref<T | null>(null) as Ref<T | null>;
  const busy = ref(false);
  const error = ref<ShownError | null>(null);

  function ask(next: T) {
    error.value = null;
    target.value = next;
  }

  function cancel() {
    if (!busy.value) {
      target.value = null;
      error.value = null;
    }
  }

  async function confirm(): Promise<boolean> {
    const current = target.value;
    if (current === null || busy.value) {
      return false;
    }
    busy.value = true;
    error.value = null;
    try {
      await run(current);
      target.value = null;
      return true;
    } catch (err) {
      error.value = toError(err);
      return false;
    } finally {
      busy.value = false;
    }
  }

  const handle: ConfirmHandle = { target, busy, error, cancel, confirm };
  return { ...handle, target, ask };
}
