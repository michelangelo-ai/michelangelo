import { useStyletron } from 'baseui';

import { useGetCellRenderer } from '#core/components/cell/use-get-cell-renderer';
import { useInterpolationResolver } from '#core/interpolation/use-interpolation-resolver';
import { resolveColumnForRow } from '../../utils/column-resolution-utils';
import { columnTooltipHOC } from './column-tooltip-hoc';
import { getResponsiveColumnWidth } from './get-responsive-column-width';

import type { TableCellProps } from './types';

export const TableCell = (props: TableCellProps) => {
  const [css, theme] = useStyletron();
  const { record, value, columnFilterValue, setColumnFilterValue } = props;
  const resolver = useInterpolationResolver();
  const column = resolver(resolveColumnForRow(props.column, record), { row: record });

  const getCellRenderer = useGetCellRenderer();
  const ColumnRenderer = getCellRenderer({ column, record, value });
  const Component = column.tooltip
    ? columnTooltipHOC(ColumnRenderer, columnFilterValue, setColumnFilterValue)
    : ColumnRenderer;

  return (
    <div className={css({ ...getResponsiveColumnWidth(theme), overflow: 'hidden' })}>
      <Component column={column} record={record} value={value} CellComponent={TableCell} />
    </div>
  );
};
