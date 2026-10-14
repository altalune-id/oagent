import type { ShownError } from '~/lib/apperror/message';
import { useScopeLabel } from '~/composables/useScopeLabel';
import { describeError, toShown } from '~/lib/apperror/message';

// One place that wires the i18n functions and scope labels into describeError.
export function useErrorMessage() {
  const { t, te } = useI18n();
  const scope = useScopeLabel();
  return (err: unknown, fallbackKey: string): ShownError => {
    const view = describeError(err, (key, params) => t(key, params ?? {}), key => te(key), t(fallbackKey), { scope });
    return toShown(view, id => t('error.ref', { id }));
  };
}
