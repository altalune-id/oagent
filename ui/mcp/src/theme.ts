// themeFor maps the host's light or dark theme onto OAgent's matching daisyUI theme, else follows the system's preference.
export function themeFor(hostTheme: unknown, prefersDark = false): 'oagent-light' | 'oagent-dark' {
  if (hostTheme === 'dark' || hostTheme === 'light') {
    return hostTheme === 'dark' ? 'oagent-dark' : 'oagent-light';
  }
  return prefersDark ? 'oagent-dark' : 'oagent-light';
}

// applyTheme sets OAgent's theme on the document from the host context.
export function applyTheme(hostTheme: unknown, root: HTMLElement = document.documentElement) {
  const prefersDark = typeof matchMedia === 'function' && matchMedia('(prefers-color-scheme: dark)').matches;
  root.dataset.theme = themeFor(hostTheme, prefersDark);
}
