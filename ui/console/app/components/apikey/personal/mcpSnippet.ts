// The .mcp.json entry for an HTTP MCP client acting with a personal key.
export function mcpSnippet(serverName: string, mcpURL: string, key: string): string {
  const entry = { type: 'http', url: mcpURL, headers: { Authorization: `Bearer ${key}` } };
  return JSON.stringify({ mcpServers: { [serverName]: entry } }, null, 2);
}
