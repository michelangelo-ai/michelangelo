import { COLOR } from '#core/components/tag/constants';

import type { TagColor } from '#core/components/tag/types';

/**
 * @description
 * Provides a default implementation for determining the color of a state tag
 * based on the value. This is used when no `stateColorMap` is provided to the
 * `StateCell` component.
 *
 * @param value - The value of the state
 * @returns The color of the state tag
 *
 * @example
 * ```ts
 * getStateColor('PIPELINE_STATE_ERROR'); // 'red'
 * getStateColor('PIPELINE_STATE_SUCCESS'); // 'green'
 * getStateColor('PIPELINE_STATE_RUNNING'); // 'blue'
 * getStateColor('PIPELINE_STATE_INVALID'); // 'gray'
 * getStateColor(''); // 'gray'
 * ```
 */
export const getStateColor = (value: string): TagColor => {
  if (!value) return COLOR.gray;
  if (value.endsWith('_ERROR')) return COLOR.red;
  if (value.endsWith('_SUCCESS')) return COLOR.green;
  if (value.endsWith('_RUNNING')) return COLOR.blue;
  if (value.endsWith('_INVALID')) return COLOR.gray;
  return COLOR.gray;
};
