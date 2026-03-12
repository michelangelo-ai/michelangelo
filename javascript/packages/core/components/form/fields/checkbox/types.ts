import type { BaseFieldProps } from '#core/components/form/fields/types';

export interface CheckboxOption {
  value: string;
  label: string;
  description?: string;
}

export interface CheckboxFieldProps extends BaseFieldProps<string[]> {
  options: CheckboxOption[];
}
