import type { KnowledgeTuning } from './agentConfig';
import type { RetrievalDefaults, RetrievalPreset } from '~/stores/knowledge';

export interface EffectiveTuning {
  topK: number;
  minScore: number;
  maxTokens: number;
}

export type TuningField = keyof EffectiveTuning;

const tolerance = 1e-6;
const same = (a: number, b: number) => Math.abs(a - b) < tolerance;

// effectiveTuning is the tuning a search runs with: each unset value takes the server's recommended one.
export function effectiveTuning(t: KnowledgeTuning, d: RetrievalDefaults): EffectiveTuning {
  return { topK: t.topK || d.topK, minScore: t.minScore ?? d.minScore, maxTokens: t.maxTokens || d.maxTokens };
}

// presetOf names the preset the tuning matches: balanced when nothing is set, else by passages and minimum relevance, or null for custom.
export function presetOf(t: KnowledgeTuning, d: RetrievalDefaults): string | null {
  if (t.topK === 0 && t.minScore === undefined) {
    return 'balanced';
  }
  const e = effectiveTuning(t, d);
  return d.presets.find(p => p.topK === e.topK && same(p.minScore, e.minScore))?.name ?? null;
}

// withPreset applies a preset, leaving values equal to the recommended ones unset, and keeps the budget.
export function withPreset(t: KnowledgeTuning, p: RetrievalPreset, d: RetrievalDefaults): KnowledgeTuning {
  return {
    topK: p.topK === d.topK ? 0 : p.topK,
    maxTokens: t.maxTokens,
    ...(same(p.minScore, d.minScore) ? {} : { minScore: p.minScore }),
  };
}

// recommendedTuning leaves every value unset, so the server's recommended values apply.
export function recommendedTuning(): KnowledgeTuning {
  return { topK: 0, maxTokens: 0 };
}

// tuningErrors lists the set values outside the server's ranges, with each range.
export function tuningErrors(t: KnowledgeTuning, d: RetrievalDefaults): Partial<Record<TuningField, [number, number]>> {
  const out: Partial<Record<TuningField, [number, number]>> = {};
  const outside = (v: number, [min, max]: [number, number]) => v < min || v > max;
  if (t.topK && outside(t.topK, d.ranges.topK)) {
    out.topK = d.ranges.topK;
  }
  if (t.minScore !== undefined && outside(t.minScore, d.ranges.minScore)) {
    out.minScore = d.ranges.minScore;
  }
  if (t.maxTokens && outside(t.maxTokens, d.ranges.maxTokens)) {
    out.maxTokens = d.ranges.maxTokens;
  }
  return out;
}

// withField reads one typed field: empty means recommended, and only a whole (or, for relevance, any) in-range number is committed.
export function withField(t: KnowledgeTuning, field: TuningField, text: string, d: RetrievalDefaults): { tuning?: KnowledgeTuning, error?: [number, number] } {
  const raw = text.trim();
  if (raw === '') {
    if (field === 'minScore') {
      const { minScore: _, ...rest } = t;
      return { tuning: rest };
    }
    return { tuning: { ...t, [field]: 0 } };
  }
  const n = Number(raw);
  const candidate = { ...t, [field]: n };
  const range = d.ranges[field];
  if (!Number.isFinite(n) || (field !== 'minScore' && !Number.isInteger(n)) || n < range[0] || tuningErrors(candidate, d)[field]) {
    return { error: range };
  }
  return { tuning: candidate };
}

// presetLabel names a preset from its locale key, or the server's name capitalized when no key exists.
export function presetLabel(name: string, t: (key: string) => string, te: (key: string) => boolean): string {
  const key = `configure.preset_${name}`;
  return te(key) ? t(key) : name.charAt(0).toUpperCase() + name.slice(1);
}
