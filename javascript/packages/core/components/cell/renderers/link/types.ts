import type { SharedCell } from '#core/components/cell/types';

export type LinkCellConfig<TRecord = unknown> = SharedCell<TRecord, string> & {
  /**
   * @description When provided, the cell will display a link to the provided url
   */
  url: string;
};
