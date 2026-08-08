import type { BaseFieldProps, SharedFieldConfig } from '#core/components/form/fields/types';

export interface CheckboxOption {
  id: string;
  label: string;
  description?: string;
}

export interface CheckboxFieldProps extends BaseFieldProps<string[]> {
  options: CheckboxOption[];
}

export type CheckboxFieldConfig<T = string[]> = SharedFieldConfig<T, string[]> &
  Pick<CheckboxFieldProps, 'options'> & {
    type: 'checkbox';
  };
