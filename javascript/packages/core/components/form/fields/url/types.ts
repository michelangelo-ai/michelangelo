import type { BaseFieldProps, SharedFieldConfig } from '#core/components/form/fields/types';

export interface UrlFieldProps extends BaseFieldProps<string> {
  /** Display label for the link; falls back to the field value if omitted */
  urlName?: string;
}

export type UrlFieldConfig<T = string> = SharedFieldConfig<T, string> &
  Pick<UrlFieldProps, 'urlName'> & {
    type: 'url';
  };
