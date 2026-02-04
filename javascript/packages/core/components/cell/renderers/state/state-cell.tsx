import { TagCell } from '#core/components/cell/renderers/tag/tag-cell';
import { getStateColor } from './get-state-color';
import { stateToString } from './state-cell-to-string';

import type { CellRendererProps } from '#core/components/cell/types';
import type { StateCellConfig } from './types';

/**
 * Cell renderer for state/status values displayed as colored tags.
 *
 * Automatically determines tag color based on state value (e.g., "running" → green,
 * "failed" → red). Supports custom color mapping via column.stateColorMap.
 * Converts state enum values to human-readable format.
 *
 * @param props.value - State value string
 * @param props.column - Column configuration with optional stateColorMap
 * @param props.record - Data record
 *
 * @example
 * ```tsx
 * // Automatic color mapping
 * { id: 'status', type: CellType.STATE }
 * // value: "PIPELINE_STATE_RUNNING" → Green tag "Running"
 *
 * // Custom color mapping
 * {
 *   id: 'status',
 *   type: CellType.STATE,
 *   stateColorMap: { custom_state: TAG_COLOR.purple }
 * }
 * ```
 */
export const StateCell = (props: CellRendererProps<string, StateCellConfig>) => {
  const { value = '', record, column } = props;
  const color = column.stateColorMap?.[value] ?? getStateColor(value);

  return (
    <TagCell
      column={{ id: 'state', color }}
      value={stateToString({ value, column })}
      record={record}
    />
  );
};

StateCell.toString = stateToString;
