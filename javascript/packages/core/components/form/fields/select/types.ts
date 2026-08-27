import type { ReactNode } from 'react';
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
  /**
   * Renders richer content for each dropdown row — multiple columns of metadata, for
   * instance — instead of the plain `option.label`.
   *
   * Only affects the dropdown menu. Selected values always render `option.label`, so a
   * multi-select's tags stay compact no matter how detailed the rows are.
   */
  getOptionContent?: (option: SelectOption<V>) => ReactNode;
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
