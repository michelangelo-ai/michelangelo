import type { BaseFieldProps } from '../types';

export interface SelectOption {
  id: string | number;
  label: string;
  disabled?: boolean;
}

export interface SelectFieldProps extends BaseFieldProps<string | string[] | number | number[]> {
  options: SelectOption[];
  clearable?: boolean;
  searchable?: boolean;
  multi?: boolean;
  creatable?: boolean;
}
