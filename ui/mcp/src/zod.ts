import { config } from 'zod';

// NOTE: the MCP Apps CSP has no 'unsafe-eval', so zod's Function('') probe would raise a violation on every load.
config({ jitless: true });
