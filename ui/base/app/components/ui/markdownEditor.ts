export type CountTone = 'ok' | 'warning' | 'error';

// countTone grades a code point count n against max: warning from 90%, error over max.
export function countTone(n: number, max: number): CountTone {
  if (n > max) {
    return 'error';
  }
  return n >= max * 0.9 ? 'warning' : 'ok';
}
