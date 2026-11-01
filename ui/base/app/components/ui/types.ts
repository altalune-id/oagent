import type { Ref } from 'vue';
import type { ShownError } from '../../lib/apperror/message';

export interface NavItem {
  key: string;
  label: string;
  to: string;
  icon?: string;
  active?: boolean;
  badge?: string | number;
}

export interface Crumb {
  label: string;
  to?: string;
}

export interface Column {
  key: string;
  label: string;
  align?: 'start' | 'end';
}

export interface KeyValueItem {
  label: string;
  value?: string;
  mono?: boolean;
  copy?: boolean;
}

export interface SecretRevealLabels {
  once: string;
  copy: string;
  copyEnv: string;
  confirm: string;
  missing: string;
}

export interface NavGroup {
  label?: string;
  items: NavItem[];
}

export interface EntityItem {
  key: string;
  label: string;
  active?: boolean;
  pinned?: boolean;
}

export interface LanguageOption {
  code: string;
  name: string;
  icon?: string;
}

export interface ThemeFamilyOption {
  id: string;
  label: string;
  theme: string;
}

export interface ConfirmHandle {
  target: Ref<unknown>;
  busy: Ref<boolean>;
  error: Ref<ShownError | null>;
  cancel: () => void;
  confirm: () => Promise<boolean>;
}

export interface StatItem {
  label: string;
  value: string | number;
}
