import type { ShownError } from '../lib/apperror/message';
import { unref } from 'vue';
import { describeError, toShown } from '../lib/apperror/message';
import { useScopeLabel } from './useScopeLabel';

// One place that wires the i18n functions and scope labels into describeError; stores call it outside setup, so it uses the global i18n, not useI18n.
export function useErrorMessage() {
  const { $i18n } = useNuxtApp();
  const scope = useScopeLabel();
  return (err: unknown, fallbackKey: string): ShownError => {
    const t = (key: string, params?: Record<string, unknown>) => $i18n.t(key, params ?? {});
    const time = (iso: string) => new Intl.DateTimeFormat(unref($i18n.locale), { timeStyle: 'medium' }).format(new Date(iso));
    const view = describeError(err, t, key => $i18n.te(key), t(fallbackKey), { scope, time });
    return toShown(view, id => t('error.ref', { id }));
  };
}
