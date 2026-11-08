export type SelectAllState = 'all' | 'some' | 'none';

// selectAllState says whether all, some or none of a group's options are on.
export function selectAllState(on: number, total: number): SelectAllState {
  return on === 0 ? 'none' : on >= total ? 'all' : 'some';
}

// withAll adds every option to the selection, or removes them all, keeping values outside the group.
export function withAll(selected: string[], options: string[], on: boolean): string[] {
  return on ? [...selected, ...options.filter(o => !selected.includes(o))] : selected.filter(s => !options.includes(s));
}
