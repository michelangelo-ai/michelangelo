import type { SharedCell } from '#core/components/cell/types';
import type { TagColor } from '#core/components/tag/types';

export type TagCellConfig = SharedCell<string> & {
  /**
   * @description The color of the tag
   * @default COLOR.gray
   */
  color?: TagColor;
};
