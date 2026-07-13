import type { BaseFieldProps } from '../types';

export type StringFieldProps =
  | (BaseFieldProps<string> & { multi?: false })
  | (BaseFieldProps<string[]> & { multi: true });
