import type { Override } from 'baseui/overrides';
import type { Cell } from '#core/components/cell/types';

export type RowProps = {
  items: RowCell[];
  record?: Record<string, unknown>;
  loading?: boolean;
  overrides?: RowOverrides;
};

export type RowCell = Cell & {
  /**
   * @description
   * If possible, hide the column when its value is empty
   *
   * @example
   * In the "Triggers" metadata row, some columns (i.e. 'Max concurrency') need
   * to be hidden when their value is empty.
   *
   * @see: https://t3.uberinternal.com/browse/MA-36479
   *
   * @default false
   */
  hideEmpty?: boolean;
};

type RowOverrides = {
  RowContainer?: Override;
  RowItemContainer?: Override;
};
