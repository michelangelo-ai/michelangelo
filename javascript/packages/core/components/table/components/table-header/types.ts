import type {
  ColumnRenderState,
  SelectableCapability,
  SortingCapability,
  VisibilityCapability,
} from '#core/components/table/types/column-types';
import type { TableData } from '#core/components/table/types/data-types';
import type { ControlledTableState } from '#core/components/table/types/table-types';

export type TableHeaderProps<T extends TableData = TableData> = {
  columns: Array<ColumnRenderState<T> & SortingCapability & VisibilityCapability>;
  setColumnOrder: ControlledTableState['setColumnOrder'];
  setColumnVisibility: ControlledTableState['setColumnVisibility'];
  enableRowSelection?: boolean;
} & Omit<SelectableCapability, 'canSelect'>;
