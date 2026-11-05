import type { ToolHint } from '~/stores/mcpServers';

// toolName is the prefixed name the model sees for a server's tool.
export function toolName(server: string, tool: string): string {
  return `${server}__${tool}`;
}

// nameParts splits a tool name after each underscore, so it can wrap there and never mid-word.
export function nameParts(name: string): string[] {
  return name.match(/[^_]*_+|[^_]+$/g) ?? [name];
}

// hintBadges maps a tool's hint to its badge text key and icon.
export const hintBadges: Record<ToolHint, { labelKey: string, icon: string }> = {
  read_only: { labelKey: 'mcptools.hint_read_only', icon: 'lucide:eye' },
  changes: { labelKey: 'mcptools.hint_changes', icon: 'lucide:pencil' },
  destructive: { labelKey: 'mcptools.hint_destructive', icon: 'lucide:triangle-alert' },
};

const reservedHeaders = ['host', 'content-length', 'transfer-encoding', 'connection', 'content-type', 'accept', 'mcp-session-id', 'mcp-protocol-version', 'te', 'trailer', 'upgrade', 'keep-alive', 'proxy-authorization', 'proxy-connection', 'expect'];

// headerNameError returns the text key for a header name the server refuses: not an HTTP token, or one oagent sets itself.
export function headerNameError(name: string): string {
  if (!/^[\w!#$%&'*+.^`|~-]+$/.test(name)) {
    return 'mcpservers.header_required';
  }
  return reservedHeaders.includes(name.toLowerCase()) ? 'mcpservers.header_reserved' : '';
}
