import type { Cell, SharedCell } from '#core/components/cell/types';

export type MultiCellConfig = SharedCell<unknown> & {
  /**
   * @description Used to render the cell with multiple lines with different values
   * e.g. First line with Pipeline name and second line with revision identifier
   *
   * @example
   * ```ts
   * {
   *  items: [
   *    { id: 'metadata.name' },
   *    { id: 'spec.revisionId' }
   *  ]
   * }
   * ```
   */
  items: Array<Cell>;
};
