import type { BaseFieldProps, SharedFieldConfig } from '../types';

export type NumberFieldProps = BaseFieldProps<number | undefined>;

export type NumberFieldConfig<T = number | undefined> = SharedFieldConfig<T, number | undefined> & {
  type: 'number';
};
