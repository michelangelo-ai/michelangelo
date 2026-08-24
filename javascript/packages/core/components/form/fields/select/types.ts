import type { BaseFieldProps, SharedFieldConfig } from '../types';

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

type SelectFieldConfigOwnProps = Pick<
  SelectFieldOwnProps<string | number>,
  'options' | 'clearable' | 'searchable' | 'creatable' | 'isLoading' | 'visibleOptionLimit'
>;

export type SingleSelectFieldConfig<T = string | number> = SharedFieldConfig<T, string | number> &
  SelectFieldConfigOwnProps & {
    type: 'select';
    multi?: false;
  };

export type MultiSelectFieldConfig<T = Array<string | number>> = SharedFieldConfig<
  T,
  Array<string | number>
> &
  SelectFieldConfigOwnProps & {
    type: 'select';
    multi: true;
  };

export type SelectFieldConfig<T = string | number> =
  | SingleSelectFieldConfig<T>
  | MultiSelectFieldConfig<T>;
