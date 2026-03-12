import type { BaseFieldProps } from '#core/components/form/fields/types';

export interface UrlFieldProps extends BaseFieldProps<string> {
  /** Display label for the link; falls back to the field value if omitted */
  urlName?: string;
}
