import type { Ref } from 'vue';
import type { ShownError } from '../../lib/apperror/message';

export interface NavItem {
  key: string;
  label: string;
  icon?: string;
  active?: boolean;
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
