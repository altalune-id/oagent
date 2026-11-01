export type ErrorKind = 'not_found' | 'forbidden' | 'failed';

// errorKind sorts an error status into the three pages people see.
export function errorKind(statusCode?: number): ErrorKind {
  if (statusCode === 404) {
    return 'not_found';
  }
  if (statusCode === 401 || statusCode === 403) {
    return 'forbidden';
  }
  return 'failed';
}
