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

export interface SelectOption {
  value: string;
  label: string;
}

export interface StatItem {
  label: string;
  value: string | number;
}
