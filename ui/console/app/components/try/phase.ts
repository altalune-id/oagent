import type { StatusTone } from '#base/components/ui/types';
import type { Phase } from './useTryChat';

const labels: Record<Phase, { key: string, tone: StatusTone }> = {
  idle: { key: 'try.phase_idle', tone: 'neutral' },
  thinking: { key: 'runstatus.running', tone: 'info' },
  streaming: { key: 'runstatus.running', tone: 'info' },
  tool: { key: 'runstatus.running', tone: 'info' },
  stopping: { key: 'try.phase_stopping', tone: 'info' },
  completed: { key: 'runstatus.completed', tone: 'success' },
  failed: { key: 'runstatus.failed', tone: 'error' },
  cancelled: { key: 'runstatus.cancelled', tone: 'neutral' },
  limit: { key: 'runstatus.limit_reached', tone: 'warning' },
  dropped: { key: 'try.phase_dropped', tone: 'warning' },
};

// phaseLabel names a Try it run phase with the run status words used on every screen.
export function phaseLabel(phase: Phase) {
  return labels[phase];
}
