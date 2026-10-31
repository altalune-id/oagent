import type { ShownError } from '../lib/apperror/message';
import { describeError, toShown } from '../lib/apperror/message';
import { useScopeLabel } from './useScopeLabel';

// One place that wires the i18n functions and scope labels into describeError.
export function useErrorMessage() {
  const { $i18n } = useNuxtApp();
  const scope = useScopeLabel();
  return (err: unknown, fallbackKey: string): ShownError => {
    const t = (key: string, params?: Record<string, unknown>) => $i18n.t(key, params ?? {});
    const view = describeError(err, t, key => $i18n.te(key), t(fallbackKey), { scope });
    return toShown(view, id => t('error.ref', { id }));
  };
}
