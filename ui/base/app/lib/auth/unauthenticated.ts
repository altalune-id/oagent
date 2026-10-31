import { Code, ConnectError } from '@connectrpc/connect';

export function isUnauthenticated(err: unknown): boolean {
  return err instanceof ConnectError && err.code === Code.Unauthenticated;
}

// Several requests can fail at once; only the first one navigates.
export function createRedirectOnce(go: (url: string) => void): (url: string) => void {
  let done = false;
  return (url) => {
    if (done) {
      return;
    }
    done = true;
    go(url);
  };
}
