import type { BaseFieldProps } from '../types';

export interface MarkdownFieldProps extends BaseFieldProps<string> {
  rows?: number;
  maxLength?: number;
}
