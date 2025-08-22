import { CellTooltipContentRenderer } from '#core/components/cell/components/tooltip/cell-tooltip-content-renderer';
import { CellTooltipWrapper } from '#core/components/cell/components/tooltip/cell-tooltip-wrapper';
import { TooltipHOCProps } from '#core/components/cell/components/tooltip/types';
import { useGetCellRenderer } from '#core/components/cell/use-get-cell-renderer';
import { useInterpolationResolver } from '#core/interpolation/use-interpolation-resolver';
import { isFilterAlreadyApplied } from './utils';

import type { TableCellProps } from './types';

export const TableCell = (props: TableCellProps) => {
  const { record, value, columnFilterValue, setColumnFilterValue } = props;
  const resolver = useInterpolationResolver();
  const column = resolver(props.column, { row: record });

  const getCellRenderer = useGetCellRenderer();
  const ColumnRenderer = getCellRenderer({ column, record, value });

  const renderedCell = <ColumnRenderer column={column} record={record} value={value} />;

  if (column.tooltip) {
    const { action } = column.tooltip;

    if (action === 'filter' && isFilterAlreadyApplied(columnFilterValue, value)) {
      return renderedCell;
    }

    const actionHandler = () => {
      if (action === 'filter' && setColumnFilterValue) {
        setColumnFilterValue([value]);
      }
    };

    return (
      <CellTooltipWrapper
        actionHandler={actionHandler}
        content={
          <CellTooltipContentRenderer
            column={column as TooltipHOCProps<unknown>['column']}
            record={record}
            value={value}
          />
        }
      >
        {renderedCell}
      </CellTooltipWrapper>
    );
  }

  return renderedCell;
};
