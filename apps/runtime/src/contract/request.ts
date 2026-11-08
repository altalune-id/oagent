import { MessageSchema, UserMessageSchema } from '@ag-ui/core/schemas';
import { z } from 'zod';

export const CONTRACT_VERSION = 2;

const ms = z.number().int().positive();
const micros = z.number().int().nonnegative();

const ModelSpec = z.strictObject({
  baseUrl: z.string().min(1),
  apiKey: z.string().min(1),
  modelId: z.string().min(1),
  maxTokens: z.number().int().positive().optional(),
});

const MCPAuth = z.strictObject({
  kind: z.enum(['none', 'bearer', 'header']),
  header: z.string().min(1).optional(),
  value: z.string().min(1).optional(),
});

const MCPServerSpec = z.strictObject({
  name: z.string().min(1),
  url: z.string().min(1),
  auth: MCPAuth,
  allowedTools: z.array(z.string().min(1)),
  connectTimeoutMs: ms,
  callTimeoutMs: ms,
});

const ToolResult = z.strictObject({
  status: z.enum(['success', 'error']),
  content: z.array(z.record(z.string(), z.unknown())),
});

const TurnInput = z.discriminatedUnion('kind', [
  z.strictObject({ kind: z.literal('message'), message: UserMessageSchema }),
  z.strictObject({
    kind: z.literal('resume'),
    responses: z.array(z.strictObject({ interruptId: z.string().min(1), response: z.unknown() })).min(1),
  }),
  z.strictObject({ kind: z.literal('continue') }),
]);

const Budget = z.strictObject({
  remainingMicros: z.number().int(),
  prices: z.strictObject({ input: micros, output: micros, cacheRead: micros, cacheWrite: micros }),
});

const TurnRequestShape = z.strictObject({
  contractVersion: z.union([z.literal(1), z.literal(CONTRACT_VERSION)]),
  ids: z.strictObject({
    run: z.string().min(1),
    thread: z.string().min(1),
    agent: z.string().min(1),
    agentVersion: z.number().int().nonnegative(),
    attempt: z.number().int().positive(),
    project: z.string().min(1),
  }),
  agent: z.strictObject({
    instructions: z.string(),
    temperature: z.number().min(0).max(2),
    maxSteps: z.number().int().nonnegative(),
    model: ModelSpec,
  }),
  mcpServers: z.array(MCPServerSpec),
  snapshot: z.record(z.string(), z.unknown()).nullable(),
  history: z.array(MessageSchema),
  input: TurnInput,
  storedResults: z.array(z.strictObject({ toolUseId: z.string().min(1), result: ToolResult })),
  limits: z.strictObject({ deadlineMs: ms, budget: Budget.nullable() }),
});

function isJsonObjectText(text: string): boolean {
  if (text === '') {
    return true;
  }
  try {
    const v: unknown = JSON.parse(text);
    return v !== null && typeof v === 'object' && !Array.isArray(v);
  } catch {
    return false;
  }
}

export const TurnRequestSchema = TurnRequestShape.superRefine((req, ctx) => {
  if (req.input.kind === 'continue' && (req.snapshot !== null || req.history.at(-1)?.role !== 'tool')) {
    ctx.addIssue({ code: 'custom', path: ['input', 'kind'], message: 'continue needs no snapshot and a history ending in a tool result' });
  }
  req.history.forEach((m, i) => {
    if (m.role !== 'assistant') {
      return;
    }
    (m.toolCalls ?? []).forEach((call, j) => {
      if (!isJsonObjectText(call.function.arguments)) {
        ctx.addIssue({ code: 'custom', path: ['history', i, 'toolCalls', j, 'function', 'arguments'], message: 'not a JSON object' });
      }
    });
  });
});

export type TurnRequest = z.infer<typeof TurnRequestSchema>;

export type Validation = { ok: true, request: TurnRequest } | { ok: false, fields: string[] };

export function validateRequest(raw: unknown): Validation {
  const parsed = TurnRequestSchema.safeParse(raw);
  if (parsed.success) {
    return { ok: true, request: parsed.data };
  }
  const fields = new Set<string>();
  for (const issue of parsed.error.issues) {
    const base = issue.path.map(String);
    if (issue.code === 'unrecognized_keys') {
      for (const key of issue.keys) {
        fields.add([...base, key].join('.'));
      }
      continue;
    }
    fields.add(base.join('.'));
  }
  return { ok: false, fields: [...fields] };
}
