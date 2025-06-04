import type { SharedCell } from '#core/components/cell/types';
import type { TagColor } from '#core/components/tag/types';

export type StateCellConfig = SharedCell<string> & {
  /**
   * @description A map of state values to their display text
   * @example
   * {
   *   'PIPELINE_STATE_BUILDING': 'Building',
   *   'PIPELINE_STATE_ERROR': 'Error'
   * }
   */
  stateTextMap?: Record<string, string>;

  /**
   * @description A map of state values to their tag colors
   * If not provided, will use the default implementation that colors based on state suffix
   * @example
   * {
   *   'PIPELINE_STATE_ERROR': 'red',
   *   'PIPELINE_STATE_SUCCESS': 'green',
   *   'PIPELINE_STATE_BUILDING': 'blue'
   * }
   */
  stateColorMap?: Record<string, TagColor>;
};
