export type MaskOptions = {
  email: boolean,
  phone: boolean,
  card: boolean,
  secrets: boolean,
  keyPrefix?: string,
};

export const DEFAULT_KEY_PREFIX = 'oagent_';

export const MASK_TOKENS = {
  email: '[EMAIL]',
  phone: '[PHONE]',
  card: '[CARD]',
  secret: '[SECRET]',
} as const;

const KEY_FAMILIES = ['sk-', 'sk_live_', 'sk_test_', 'rk_live_', 'rk_test_', 'key_', 'ghp_', 'gho_', 'ghs_', 'ghu_', 'github_pat_', 'xox[abprs]-'];
const AWS_KEY = /(?<![A-Za-z0-9])(?:AKIA|ASIA)[A-Z0-9]{16}(?![A-Za-z0-9])/g;
const JWT = /(?<![\w-])eyJ[\w-]{8,}\.[\w-]{8,}\.[\w-]{8,}/g;
const BEARER = /\b([Bb]earer[ \t]{1,4})[\w~+/.=-]{16,}/g;
const BASIC = /\b([Bb]asic[ \t]{1,4})[A-Za-z0-9+/]{12,}={0,2}/g;
const PEM = /-----BEGIN [A-Z ]{0,40}PRIVATE KEY-----[A-Za-z0-9+/=\s]{0,16384}(?:-----END [A-Z ]{0,40}PRIVATE KEY-----)?/g;
const GOOGLE_KEY = /(?<![\w-])AIza[\w-]{35}(?![\w-])/g;
const EMAIL = /(?<![\w.%+-])[\w.%+-]{1,64}@[a-z0-9-]{1,63}(?:\.[a-z0-9-]{1,63})*\.[a-z]{2,24}(?![a-z0-9-])/gi;
const CARD = /(?<![\d+])[1-9](?:[ -]?\d){12,18}(?!\d)/g;
const PHONE = /(?<![\w+])(?:[+0]\d(?:[ .-]?\d){7,13}|62[ .-]?8(?:[ .-]?\d){7,12})(?!\d)/g;

function escapeRegExp(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function keyPattern(keyPrefix: string): RegExp {
  return new RegExp(`(?<![\\w-])(?:${[...KEY_FAMILIES, escapeRegExp(keyPrefix)].join('|')})[\\w-]{16,}`, 'g');
}

function luhn(digits: string): boolean {
  let sum = 0;
  for (let i = 0; i < digits.length; i++) {
    let d = Number(digits[digits.length - 1 - i]);
    if (i % 2 === 1) {
      d *= 2;
      if (d > 9) {
        d -= 9;
      }
    }
    sum += d;
  }
  return sum % 10 === 0;
}

export function maskText(text: string, options: MaskOptions): string {
  let out = text;
  if (options.secrets) {
    out = out
      .replace(PEM, MASK_TOKENS.secret)
      .replace(keyPattern(options.keyPrefix || DEFAULT_KEY_PREFIX), MASK_TOKENS.secret)
      .replace(GOOGLE_KEY, MASK_TOKENS.secret)
      .replace(AWS_KEY, MASK_TOKENS.secret)
      .replace(JWT, MASK_TOKENS.secret)
      .replace(BEARER, `$1${MASK_TOKENS.secret}`)
      .replace(BASIC, `$1${MASK_TOKENS.secret}`);
  }
  if (options.email) {
    out = out.replace(EMAIL, MASK_TOKENS.email);
  }
  if (options.card) {
    out = out.replace(CARD, match => (luhn(match.replace(/\D/g, '')) ? MASK_TOKENS.card : match));
  }
  if (options.phone) {
    out = out.replace(PHONE, MASK_TOKENS.phone);
  }
  return out;
}
