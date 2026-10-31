import { fileURLToPath } from 'node:url';

const at = (path: string) => fileURLToPath(new URL(path, import.meta.url));

export const baseAliases: Record<string, string> = {
  '#base': at('./app'),
  '#gen': at('./gen'),
  '#test': at('./test'),
  '@shared/scope': at('../../scope'),
  '@shared/apperror': at('../../apperror'),
};
