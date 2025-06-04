import { TagCell } from '#core/components/cell/renderers/tag/tag-cell';
import { getStateColor } from './get-state-color';
import { stateToString } from './state-cell-to-string';

import type { CellRendererProps } from '#core/components/cell/types';
import type { StateCellConfig } from './types';

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
