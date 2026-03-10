import type { BaseFieldProps } from '../types';

export interface SelectOption<V = string | number> {
  id: V;
  label: string;
  disabled?: boolean;
}

export interface SelectFieldProps<V = string | number> extends BaseFieldProps<V | V[]> {
  options: SelectOption<V>[];
  clearable?: boolean;
  searchable?: boolean;
  multi?: boolean;
  creatable?: boolean;
  isLoading?: boolean;
  maxOptions?: number;
}
