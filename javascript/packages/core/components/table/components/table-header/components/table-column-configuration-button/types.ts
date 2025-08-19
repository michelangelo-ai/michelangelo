import { ColumnRenderState, VisibilityCapability } from '#core/components/table/types/column-types';
import { TableData } from '#core/components/table/types/data-types';

import type { ControlledTableState } from '#core/components/table/types/table-types';

export interface TableColumnConfigurationButtonProps<T extends TableData = TableData> {
  columns: Array<ConfigurableColumn<T>>;
  setColumnOrder: ControlledTableState['setColumnOrder'];
  setColumnVisibility: ControlledTableState['setColumnVisibility'];
}

export type ConfigurableColumn<T extends TableData = TableData> = ColumnRenderState<T> &
  VisibilityCapability;
