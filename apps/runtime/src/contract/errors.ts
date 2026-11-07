export class TurnRefusedError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string) {
    super(code);
    this.name = 'TurnRefusedError';
    this.status = status;
    this.code = code;
  }
}
