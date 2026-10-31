import type { Timestamp } from '@bufbuild/protobuf/wkt';
import { timestampDate } from '@bufbuild/protobuf/wkt';

export type DateStyle = 'date' | 'moment' | 'time';

export interface DateFormat {
  empty?: string;
  style?: DateStyle;
}

const styles: Record<DateStyle, Intl.DateTimeFormatOptions> = {
  date: { dateStyle: 'medium' },
  moment: { dateStyle: 'medium', timeStyle: 'short' },
  time: { timeStyle: 'short' },
};

// formatDate renders a day (date), a day and time (moment) or a time of day in locale, or empty when there is no date.
export function formatDate(d: Date | Timestamp | undefined | null, locale: string, { empty = '', style = 'date' }: DateFormat = {}): string {
  if (!d) {
    return empty;
  }
  return new Intl.DateTimeFormat(locale, styles[style]).format(d instanceof Date ? d : timestampDate(d));
}
