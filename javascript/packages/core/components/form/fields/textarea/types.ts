import type { BaseFieldProps } from '../types';

export interface TextareaFieldProps extends BaseFieldProps {
  rows?: number;
  maxLength?: number;
}

export interface MaxLengthLabelEnhancerProps {
  maxLength: number;
  currentLength: number;
}
