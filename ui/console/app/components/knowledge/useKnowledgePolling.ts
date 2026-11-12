import { useKnowledgeStore } from '~/stores/knowledge';

// useKnowledgePolling owns one page's status polling: it starts once mounted, restarts on demand, and stops for good on unmount.
export function useKnowledgePolling(projectId: string, sourceId = '') {
  const store = useKnowledgeStore();
  let stop: (() => void) | undefined;
  let mounted = false;

  function start() {
    if (!mounted) {
      return;
    }
    stop?.();
    stop = store.startPolling(projectId, sourceId);
  }

  onMounted(() => {
    mounted = true;
    start();
  });
  onBeforeUnmount(() => {
    mounted = false;
    stop?.();
    stop = undefined;
  });

  return { start };
}
