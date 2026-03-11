import type { BaseFieldProps } from '../types';

export interface SelectOption<V = string | number> {
  id: V;
  label: string;
  disabled?: boolean;
}

interface SelectFieldOwnProps<V> {
  options: SelectOption<V>[];
  clearable?: boolean;
  searchable?: boolean;
  creatable?: boolean;
  isLoading?: boolean;
  /**
   * Limit the number of visible options in the dropdown.
   * By default there is no limit.
   */
  visibleOptionLimit?: number;
}

export type SelectFieldProps<V = string | number> =
  | (SelectFieldOwnProps<V> & BaseFieldProps<V> & { multi?: false })
  | (SelectFieldOwnProps<V> & BaseFieldProps<V[]> & { multi: true });
