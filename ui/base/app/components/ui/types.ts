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
  mono?: boolean;
}

export interface Column {
  key: string;
  label: string;
  align?: 'start' | 'end';
  mono?: boolean;
  muted?: boolean;
  hideLabel?: boolean;
  sortable?: boolean;
}

export interface TableSort {
  key: string;
  dir: 'ascending' | 'descending';
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
  mono?: boolean;
  badge?: string;
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
  unknown?: string;
}

export type StatusTone = 'success' | 'warning' | 'error' | 'info' | 'neutral';

export interface PaletteItem {
  key: string;
  label: string;
  to: string;
  group?: string;
  mono?: boolean;
}

export interface TopBarPlace {
  label: string;
  badge?: string;
}
