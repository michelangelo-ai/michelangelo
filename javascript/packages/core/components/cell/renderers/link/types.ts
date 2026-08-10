import type { SharedCell } from '#core/components/cell/types';

export type LinkCellConfig<TRecord = unknown> = SharedCell<TRecord, string> & {
  /**
   * @description When provided, the cell will display a link to the provided url
   */
  url: string;

  /**
   * @description Invoked with the row's record when the rendered link is clicked, before
   * navigation occurs. Lets a consumer attach click-tracking or other side effects without
   * this cell renderer knowing anything about what the callback does.
   */
  onClick?: (record: unknown) => void;
};
