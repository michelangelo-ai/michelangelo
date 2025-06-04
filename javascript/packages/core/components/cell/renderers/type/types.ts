import type { SharedCell } from '#core/components/cell/types';

export type TypeCellConfig = SharedCell<string> & {
  /**
   * @description A map of type values to their display text
   */
  typeTextMap?: Record<string, string>;
};
