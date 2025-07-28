import { useGetCellRenderer } from '#core/components/cell/use-get-cell-renderer';
import { useInterpolationResolver } from '#core/interpolation/use-interpolation-resolver';

import type { CellRendererProps } from '#core/components/cell/types';
import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { TableData } from '#core/components/table/types/data-types';

export const TableCell = (props: CellRendererProps<TableData, ColumnConfig>) => {
  const { record, value } = props;
  const resolver = useInterpolationResolver();
  const column = resolver(props.column, { row: record });

  const getCellRenderer = useGetCellRenderer();
  const ColumnRenderer = getCellRenderer({ column, record, value });

  return <ColumnRenderer column={column} record={record} value={value} />;
};
