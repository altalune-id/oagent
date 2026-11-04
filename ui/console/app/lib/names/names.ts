// nameTaken reports whether another item already uses name, ignoring case and surrounding spaces.
export function nameTaken(items: { id: string, name: string }[], name: string, exceptId = ''): boolean {
  const wanted = name.trim().toLowerCase();
  return items.some(i => i.id !== exceptId && i.name.trim().toLowerCase() === wanted);
}

// describedBy joins the ids of the hints and errors that are on screen, or undefined when none are.
export function describedBy(...ids: (string | false | undefined)[]): string | undefined {
  return ids.filter(Boolean).join(' ') || undefined;
}
