import type { Align } from 'baseui/radio';
import type { BaseFieldProps } from '../types';

export interface RadioOption {
  value: string | boolean;
  label: string;
  disabled?: boolean;
}

export interface RadioFieldProps extends BaseFieldProps {
  options: RadioOption[];
  align?: Align;
}
