import type { BaseFieldProps } from '#core/components/form/fields/types';

export interface CheckboxOption {
  id: string;
  label: string;
  description?: string;
}

export interface CheckboxFieldProps extends BaseFieldProps<string[]> {
  options: CheckboxOption[];
}
