import type { McpTool, ToolHint } from '~/stores/mcpServers';

const toolNamePattern = /^[\w-]{1,64}$/;

// toolName is the prefixed name the model sees for a server's tool.
export function toolName(server: string, tool: string): string {
  return `${server}__${tool}`;
}

// nameParts splits a tool name after each underscore, so it can wrap there and never mid-word.
export function nameParts(name: string): string[] {
  return name.match(/[^_]*_+|[^_]+$/g) ?? [name];
}

// grantable mirrors the grant-time check on the prefixed name the model sees.
export function grantable(server: string, tool: McpTool): boolean {
  return toolNamePattern.test(toolName(server, tool.name));
}

// hintBadges maps a tool's hint to its badge text key and icon.
export const hintBadges: Record<ToolHint, { labelKey: string, icon: string }> = {
  read_only: { labelKey: 'mcptools.hint_read_only', icon: 'lucide:eye' },
  changes: { labelKey: 'mcptools.hint_changes', icon: 'lucide:pencil' },
  destructive: { labelKey: 'mcptools.hint_destructive', icon: 'lucide:triangle-alert' },
};

// serverNameError returns the text key of the first rule a new server name breaks, or '' when it is valid.
export function serverNameError(name: string): string {
  if (!name) {
    return 'mcpservers.name_required';
  }
  if (name.includes('__')) {
    return 'mcpservers.name_double_underscore';
  }
  return /^[\w-]{1,32}$/.test(name) ? '' : 'mcpservers.name_chars';
}
