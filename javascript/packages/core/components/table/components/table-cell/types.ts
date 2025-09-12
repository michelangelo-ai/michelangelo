import type { TooltipHOCProps } from '#core/components/cell/components/tooltip/types';
import type { CellRendererProps } from '#core/components/cell/types';
import type { TableRow } from '#core/components/table/components/table-body/types';
import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { TableData } from '#core/components/table/types/data-types';

export interface TableCellProps<T extends TableData = TableData>
  extends CellRendererProps<T, ColumnConfig> {
  columnFilterValue?: unknown;
  row: TableRow<T>;
  setColumnFilterValue?: (value: unknown) => void;
}

export interface ColumnTooltipContentRendererProps<T = unknown> extends TooltipHOCProps<T> {
  row: TableRow<T>;
}
