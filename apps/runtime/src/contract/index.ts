import type { TurnRequest } from './request.ts';

export { TurnRefusedError } from './errors.ts';
export { checkImages, inputParts } from './images.ts';
export type { ImageMime, InputImage, InputPart } from './images.ts';
export { CONTRACT_VERSION, TurnRequestSchema, validateRequest } from './request.ts';
export type { TurnRequest, Validation } from './request.ts';

export type Emit = (event: Record<string, unknown>) => Promise<void>;

export type RunTurn = (request: TurnRequest, emit: Emit, signal: AbortSignal) => Promise<void>;
