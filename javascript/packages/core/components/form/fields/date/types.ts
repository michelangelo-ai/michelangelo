import type { BaseFieldProps } from '../types';

export interface DateFieldProps extends BaseFieldProps<string, Date | null> {
  /** @default DateFormat.ISO_DATE_STRING */
  dateFormat?: DateFormat;
  /**
   * Restricts selectable dates to today or earlier
   * @default false
   */
  noFutureDate?: boolean;
}

export enum DateFormat {
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
