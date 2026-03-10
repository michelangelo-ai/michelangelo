import type { Align } from 'baseui/radio';
import type { BaseFieldProps } from '../types';

export interface RadioOption {
  value: string | boolean;
  label: string;
  /** When any option has a non-empty description, all options render as cards instead of inline radios. */
  description?: string;
  disabled?: boolean;
}

export interface RadioFieldProps extends BaseFieldProps<string | boolean> {
  options: RadioOption[];
  align?: Align;
}
