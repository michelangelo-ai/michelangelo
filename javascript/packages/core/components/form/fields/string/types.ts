import type { SharedFieldConfig } from '../types';
import type { BaseFieldProps } from '../types';

export type SingleStringFieldProps = BaseFieldProps<string> & {
  multi?: false;
  /**
   * Limits input length and displays a character counter in the label row.
   * When `labelEndEnhancer` is also provided, the counter appears first
   * followed by the enhancer content.
   */
  maxLength?: number;
};
export type MultiStringFieldProps = BaseFieldProps<string[]> & { multi: true };

export type StringFieldProps = SingleStringFieldProps | MultiStringFieldProps;

export type SingleStringFieldConfig<T = string> = SharedFieldConfig<T, string> & {
  type: 'string';
  multi?: false;
};

export type MultiStringFieldConfig<T = string[]> = SharedFieldConfig<T, string[]> & {
  type: 'string';
  multi: true;
};

export type StringFieldConfig<T = string> = SingleStringFieldConfig<T> | MultiStringFieldConfig<T>;
