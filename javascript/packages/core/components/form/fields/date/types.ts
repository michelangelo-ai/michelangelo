import type { BaseFieldProps } from '../types';

export interface DateFieldProps extends BaseFieldProps<string> {
  /** @default DATE_FORMAT.ISO_DATE_STRING */
  dateFormat?: DATE_FORMAT;
  /** Restricts selectable dates to today or earlier */
  noFutureDate?: boolean;
}

export enum DATE_FORMAT {
  /**
   * Anticipates date as epoch seconds string. String is artifact of int64 protobuf fields
   * being typed as Strings in TypeScript
   */
  EPOCH_SECONDS = 'epoch',
  /**
   * Anticipates date as ISO string.
   */
  ISO_DATE_STRING = 'iso',
}
