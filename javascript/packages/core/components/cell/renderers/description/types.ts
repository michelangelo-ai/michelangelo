import type { SharedCell } from '#core/components/cell/types';
import type { DescriptionHierarchy } from './constants';

export type DescriptionCellConfig = SharedCell & {
  /**
   * @description Used to control cell styling – e.g. color, font-size, etc.
   */
  hierarchy?: DescriptionHierarchy;
};
