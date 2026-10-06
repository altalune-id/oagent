export interface NavItem {
  key: string
  label: string
  active?: boolean
}

export interface SelectOption {
  value: string
  label: string
}

export interface StatItem {
  label: string
  value: string | number
}
