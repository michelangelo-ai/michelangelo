import type { FormControlOverrides } from 'baseui/form-control';

export interface FormControlProps {
  label?: string;
  required?: boolean;
  description?: string;
  caption?: string;
  error?: string;
  counter?: {
    length: number;
    maxLength: number;
  };
  overrides?: FormControlOverrides;
  children: React.ReactNode;
}
