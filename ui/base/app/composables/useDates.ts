import type { Timestamp } from '@bufbuild/protobuf/wkt';
import { formatDate } from '../lib/format/date';
import { relativeTime } from '../lib/tokens/keyStatus';

// useDates formats days, moments and "how long ago" in the current locale, counting from when the page opened.
export function useDates(empty = '') {
  const { locale } = useI18n();
  const now = new Date();
  return {
    date: (d?: Date | Timestamp | null) => formatDate(d, locale.value, { empty }),
    moment: (d?: Date | Timestamp | null) => formatDate(d, locale.value, { empty, style: 'moment' }),
    when: (d?: Date) => relativeTime(d, now, locale.value),
  };
}
