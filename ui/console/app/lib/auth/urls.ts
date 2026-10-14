// apiUrl is runtimeConfig.public.apiUrl ('' in dev means '/api'); Go rebases it per http.basePath, so its parent is the server root.
export function siblingURL(apiUrl: string, path: '/login' | '/logout' | '/mcp' | `/static/${string}`): string {
  const api = (apiUrl || '/api').replace(/\/+$/, '');
  return api.replace(/\/api$/, '') + path;
}

export function loginURL(apiUrl: string, current: { pathname: string, search: string, hash: string }): string {
  const returnTo = current.pathname + current.search + current.hash;
  return `${siblingURL(apiUrl, '/login')}?return_to=${encodeURIComponent(returnTo)}`;
}

export function mcpURL(apiUrl: string, origin: string): string {
  return new URL(siblingURL(apiUrl, '/mcp'), origin).href;
}

// staticURL is a file the server embeds for the sign-in pages (logo, favicons), next to the API like siblingURL.
export function staticURL(apiUrl: string, file: string): string {
  return siblingURL(apiUrl, `/static/${file}`);
}
