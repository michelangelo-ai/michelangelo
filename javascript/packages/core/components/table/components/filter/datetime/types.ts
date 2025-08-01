/**
 * Datetime filter value representing a date range with operation details
 */
export interface DatetimeFilterValue {
  operation: string;
  range: Date[];
  selection: number[];
  description: string;
  exclude: boolean;
}
