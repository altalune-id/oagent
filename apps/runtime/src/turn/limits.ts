export type Prices = {
  input: number,
  output: number,
  cacheRead: number,
  cacheWrite: number,
};

export type Budget = {
  remainingMicros: number,
  prices: Prices,
  helperPrices?: Prices,
};

export type TokenUsage = {
  inputTokens: number,
  outputTokens: number,
  cacheReadInputTokens: number,
  cacheWriteInputTokens: number,
};

export type TurnUsage = TokenUsage & {
  estimated: boolean,
  helper?: TokenUsage,
};

export const LIMIT_REPLY = 'This agent reached its spending limit for this conversation.';
export const BUDGET_REFUSAL = 'DENIED: budget exhausted';
const CHARS_PER_TOKEN = 4;
export const DEFAULT_MAX_STEPS = 20;

export function stepLimit(maxSteps: number): number {
  return maxSteps > 0 ? maxSteps : DEFAULT_MAX_STEPS;
}

export function emptyUsage(): TurnUsage {
  return { inputTokens: 0, outputTokens: 0, cacheReadInputTokens: 0, cacheWriteInputTokens: 0, estimated: false };
}

function classMicros(tokens: number, pricePerMillion: number): number {
  return Math.ceil((tokens * pricePerMillion) / 1_000_000);
}

export function costMicros(usage: TokenUsage, prices: Prices): number {
  return classMicros(usage.inputTokens, prices.input)
    + classMicros(usage.outputTokens, prices.output)
    + classMicros(usage.cacheReadInputTokens, prices.cacheRead)
    + classMicros(usage.cacheWriteInputTokens, prices.cacheWrite);
}

export const EXTRACT_OUTPUT_TOKENS = 1500;

function spentMicros(usage: TurnUsage, budget: Budget): number {
  const helper = usage.helper ? costMicros(usage.helper, budget.helperPrices ?? budget.prices) : 0;
  return costMicros(usage, budget.prices) + helper;
}

export function overBudget(usage: TurnUsage, budget: Budget | null): boolean {
  return budget !== null && spentMicros(usage, budget) >= budget.remainingMicros;
}

export function extractionFits(usage: TurnUsage, budget: Budget | null, inputChars: number): boolean {
  if (budget === null) {
    return true;
  }
  const estimate = { inputTokens: Math.ceil(inputChars / CHARS_PER_TOKEN), outputTokens: EXTRACT_OUTPUT_TOKENS, cacheReadInputTokens: 0, cacheWriteInputTokens: 0 };
  return spentMicros(usage, budget) + costMicros(estimate, budget.helperPrices ?? budget.prices) < budget.remainingMicros;
}

export function addHelperUsage(usage: TurnUsage, call: TurnUsage): void {
  if (call.inputTokens + call.outputTokens + call.cacheReadInputTokens + call.cacheWriteInputTokens === 0) {
    return;
  }
  const helper = usage.helper ??= { inputTokens: 0, outputTokens: 0, cacheReadInputTokens: 0, cacheWriteInputTokens: 0 };
  helper.inputTokens += call.inputTokens;
  helper.outputTokens += call.outputTokens;
  helper.cacheReadInputTokens += call.cacheReadInputTokens;
  helper.cacheWriteInputTokens += call.cacheWriteInputTokens;
  usage.estimated ||= call.estimated;
}

export class UsageMeter {
  readonly usage = emptyUsage();
  private callOpen = false;
  private inputChars = 0;
  private outputChars = 0;

  startCall(requestChars: number): void {
    this.closeCutCall();
    this.callOpen = true;
    this.inputChars = requestChars;
    this.outputChars = 0;
  }

  addInput(chars: number): void {
    this.inputChars += chars;
  }

  addOutput(text: string): void {
    this.outputChars += text.length;
  }

  addMetadata(u: { inputTokens: number, outputTokens: number, cacheReadInputTokens?: number, cacheWriteInputTokens?: number }): void {
    this.usage.inputTokens += u.inputTokens;
    this.usage.outputTokens += u.outputTokens;
    this.usage.cacheReadInputTokens += u.cacheReadInputTokens ?? 0;
    this.usage.cacheWriteInputTokens += u.cacheWriteInputTokens ?? 0;
    this.callOpen = false;
  }

  closeCutCall(): void {
    if (!this.callOpen) {
      return;
    }
    this.usage.inputTokens += Math.ceil(this.inputChars / CHARS_PER_TOKEN);
    this.usage.outputTokens += Math.ceil(this.outputChars / CHARS_PER_TOKEN);
    this.usage.estimated = true;
    this.callOpen = false;
  }

  closeFailedCall(): void {
    if (this.callOpen && this.outputChars === 0) {
      this.callOpen = false;
      return;
    }
    this.closeCutCall();
  }
}
