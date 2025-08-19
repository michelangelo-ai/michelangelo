import { arrayMove } from 'baseui/dnd-list';

import { TableData } from '#core/components/table/types/data-types';

import type {
  ColumnOrderState,
  ControlledTableState,
} from '#core/components/table/types/table-types';
import type { ConfigurableColumn } from './types';

/**
 * Handles BaseUI dnd-list changes for column reordering and visibility toggling.
 *
 * BaseUI uses -1 as a special identifier for toggle visibility operations,
 * while positive indices represent reordering operations.
 */
export function createColumnListChangeHandler<T extends TableData = TableData>(
  columns: ConfigurableColumn<T>[],
  setColumnOrder: ControlledTableState['setColumnOrder'],
  setColumnVisibility: ControlledTableState['setColumnVisibility']
) {
  return (state: { oldIndex: number; newIndex: number }) => {
    const wasToggledHidden = state.newIndex === -1;

    // +1 to account for sliced first column
    const oldIndex = state.oldIndex + 1;
    const newIndex = state.newIndex + 1;

    if (wasToggledHidden) {
      const column = columns[oldIndex];
      setColumnVisibility((old) => ({
        ...old,
        [column.id]: !column.isVisible,
      }));
    } else {
      setColumnOrder(
        arrayMove(
          columns.map((c) => c.id),
          oldIndex,
          newIndex
        ) as ColumnOrderState
      );
    }
  };
}
