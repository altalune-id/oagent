type Translate = (key: string, params?: Record<string, unknown>) => string;

// shownName shows a person's name, or "Deleted person" once their data was deleted.
export function shownName(name: string | undefined, deleted: boolean | undefined, t: Translate): string {
  return deleted ? t('endusers.deleted_person') : name ?? '';
}
