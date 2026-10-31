import type { ConfirmHandle } from '../components/ui/types';
import { confirmHandle } from '../components/ui/confirmHandle';
import { useErrorMessage } from './useErrorMessage';

export type { ConfirmHandle };

// useConfirm runs one destructive action on a target after a confirm, keeping the confirm open on a refusal.
export function useConfirm<T>(run: (target: T) => Promise<void>, fallbackKey: string) {
  const errorMessage = useErrorMessage();
  return confirmHandle(run, err => errorMessage(err, fallbackKey));
}
