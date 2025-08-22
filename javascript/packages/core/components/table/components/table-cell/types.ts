import type { CellRendererProps } from '#core/components/cell/types';
import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { TableData } from '#core/components/table/types/data-types';

export interface TableCellProps extends CellRendererProps<TableData, ColumnConfig> {
  columnFilterValue?: unknown;
  setColumnFilterValue?: (value: unknown) => void;
}
