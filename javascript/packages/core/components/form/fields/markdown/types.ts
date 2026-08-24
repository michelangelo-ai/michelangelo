import type { BaseFieldProps, SharedFieldConfig } from '../types';

export interface MarkdownFieldProps extends BaseFieldProps<string> {
  rows?: number;
  /**
   * Limits input length and displays a character counter in the label row.
   * When `labelEndEnhancer` is also provided, the counter appears first
   * followed by the enhancer content.
   */
  maxLength?: number;
}

export type MarkdownFieldConfig<T = string> = SharedFieldConfig<T, string> &
  Pick<MarkdownFieldProps, 'rows' | 'maxLength'> & {
    type: 'markdown';
  };
