import { formatDate } from '#base/lib/format/date';

// chatTime shows the time alone for today and the date with the time for older messages, in locale.
export function chatTime(d: Date, locale: string, now = new Date()): string {
  const today = d.toDateString() === now.toDateString();
  return formatDate(d, locale, { style: today ? 'time' : 'moment' });
}

// dayBreaks maps each dated item that starts a new day to that day's label: "today" text or the date.
export function dayBreaks(items: { id: number, at?: Date }[], locale: string, today: string, now = new Date()): Map<number, string> {
  const out = new Map<number, string>();
  let last = '';
  for (const item of items) {
    if (!item.at) {
      continue;
    }
    const day = item.at.toDateString();
    if (day !== last) {
      out.set(item.id, day === now.toDateString() ? today : formatDate(item.at, locale));
      last = day;
    }
  }
  return out;
}
