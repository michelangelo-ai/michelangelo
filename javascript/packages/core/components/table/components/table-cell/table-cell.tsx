import { useStyletron } from 'baseui';

import { useCellStyles } from '#core/components/cell/hooks';
import { useGetCellRenderer } from '#core/components/cell/use-get-cell-renderer';
import { useInterpolationResolver } from '#core/interpolation/use-interpolation-resolver';
import { resolveColumnForRow } from '../../utils/column-resolution-utils';
import { columnTooltipHOC } from './column-tooltip-hoc';
import { getResponsiveColumnWidth } from './get-responsive-column-width';

import type { TableCellProps } from './types';

export const TableCell = <T = unknown,>(props: TableCellProps<T>) => {
  const [css, theme] = useStyletron();
  const { row, record, value, columnFilterValue, setColumnFilterValue } = props;
  const resolver = useInterpolationResolver();
  const column = resolver(resolveColumnForRow(props.column, record), { row: record });
  const style = useCellStyles({ record, style: column.style });

  const getCellRenderer = useGetCellRenderer();
  const ColumnRenderer = getCellRenderer({ column, record, value });
  const Component = column.tooltip
    ? columnTooltipHOC<T>(ColumnRenderer, row, columnFilterValue, setColumnFilterValue)
    : ColumnRenderer;

  return (
    <div className={css({ ...getResponsiveColumnWidth(theme), ...style })}>
      <Component column={column} record={record} value={value} CellComponent={TableCell} />
    </div>
  );
};
