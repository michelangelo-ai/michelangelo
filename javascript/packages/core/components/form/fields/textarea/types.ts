import type { BaseFieldProps } from '../types';

export interface TextareaFieldProps extends BaseFieldProps<string> {
  rows?: number;
  /**
   * Limits input length and displays a character counter in the label row.
   * When `labelEndEnhancer` is also provided, the counter appears first
   * followed by the enhancer content.
   */
  maxLength?: number;
}

export interface MaxLengthLabelEnhancerProps {
  maxLength: number;
  currentLength: number;
}
