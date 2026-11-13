import type { Model } from '@strands-agents/sdk';
import type { RunTurn, TurnRequest } from '../contract/index.ts';
import type { Logger } from '../log.ts';
import type { Models } from '../models/index.ts';
import type { TurnDeps, TurnOutcome } from './execute.ts';
import { checkImages } from '../contract/index.ts';
import { approvalSpecs, checkApprovalReady, interventionsFor } from '../interventions/index.ts';
import { StreamMapper, toAgUiMessages } from './agui.ts';
import { executeTurn } from './execute.ts';
import { emptyUsage, LIMIT_REPLY } from './limits.ts';
import { checkSnapshot } from './snapshot.ts';

export const HEARTBEAT_MS = 15_000;

const ERROR_TEXT: Record<string, string> = {
  deadline: 'turn deadline reached',
  model_error: 'model call failed',
  runtime_error: 'runtime error',
};

export interface RunTurnDeps {
  models: Models;
  log: Logger;
  modelFor?: (request: TurnRequest) => Model;
  openMcp?: TurnDeps['openMcp'];
}

function runtimeFailure(): TurnOutcome {
  return {
    status: 'failed',
    stopReason: 'error',
    snapshot: null,
    usage: emptyUsage(),
    newMessages: [],
    interrupts: [],
    notes: [],
    error: { code: 'runtime_error' },
  };
}

export function createRunTurn(deps: RunTurnDeps): RunTurn {
  return async (request, emit, signal) => {
    checkSnapshot(request);
    checkImages(request);
    checkApprovalReady(request, interventionsFor(approvalSpecs(request)));
    const model = deps.modelFor?.(request) ?? deps.models.model(request.ids.project, request.agent.model, request.agent.temperature);
    const mapper = new StreamMapper(emit);
    const ids = { threadId: request.ids.thread, runId: request.ids.run };
    await emit({ type: 'RUN_STARTED', ...ids });
    const beat = setInterval(() => {
      void emit({ type: 'OAGENT_HEARTBEAT' });
    }, HEARTBEAT_MS);
    let outcome: TurnOutcome;
    try {
      outcome = await executeTurn(request, signal, {
        model,
        log: deps.log,
        models: deps.models,
        ...(deps.openMcp && { openMcp: deps.openMcp }),
        onEvent: e => mapper.onEvent(e),
        onToolResult: r => mapper.onToolResult(r),
      });
    } catch (error) {
      deps.log.error({ run: request.ids.run, error }, 'turn crashed');
      outcome = runtimeFailure();
    } finally {
      clearInterval(beat);
    }
    deps.log.info({
      run: request.ids.run,
      status: outcome.status,
      stopReason: outcome.stopReason,
      inputTokens: outcome.usage.inputTokens,
      outputTokens: outcome.usage.outputTokens,
      parts: outcome.newMessages.filter(m => m.role === 'assistant').map(m => m.content.map(b => b.type).join('+') || 'empty'),
    }, 'turn outcome');
    if (outcome.stopReason === 'budget') {
      await mapper.reply(LIMIT_REPLY);
    }
    if (outcome.error) {
      await emit({ type: 'RUN_ERROR', message: ERROR_TEXT[outcome.error.code] ?? 'turn failed', code: outcome.error.code });
    } else {
      await emit({ type: 'RUN_FINISHED', ...ids });
    }
    await emit({
      type: 'OAGENT_TERMINAL',
      status: outcome.status,
      stopReason: outcome.stopReason,
      snapshot: outcome.snapshot,
      usage: outcome.usage,
      messagesAppended: toAgUiMessages(request, outcome.newMessages),
      interrupts: outcome.interrupts,
      notes: outcome.notes,
      ...(outcome.error && { error: outcome.error }),
      ...(outcome.memory && { memory: outcome.memory }),
    });
  };
}
