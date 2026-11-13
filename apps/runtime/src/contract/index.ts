import type { TurnRequest } from './request.ts';

export { argsHash, canonicalJson } from './canonical.ts';
export { TurnRefusedError } from './errors.ts';
export { checkImages, inputParts } from './images.ts';
export type { ImageMime, InputImage, InputPart } from './images.ts';
export { APPROVAL_CONTRACT_VERSION, ApprovalSpecSchema, CONTRACT_VERSION, IMAGE_CONTRACT_VERSION, MAX_FACT_RUNES, MAX_MEMORY_FACTS, MAX_PASSAGE_CHARS, MAX_PASSAGES, MCPAuthKind, MemoryExtract, ModelProvider, ToolResultStatus, TurnRequestSchema, validateRequest } from './request.ts';
export type { ApprovalSpec, KnowledgePassage, MemoryFact, TurnRequest, Validation } from './request.ts';

export type Emit = (event: Record<string, unknown>) => Promise<void>;

export type RunTurn = (request: TurnRequest, emit: Emit, signal: AbortSignal) => Promise<void>;
