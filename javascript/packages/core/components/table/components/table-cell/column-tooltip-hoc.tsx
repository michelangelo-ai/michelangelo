import { CellTooltipWrapper } from '#core/components/cell/components/tooltip/cell-tooltip-wrapper';
import { ColumnTooltipContentRenderer } from './column-tooltip-content-renderer';
import { isFilterAlreadyApplied } from './utils';

import type { TooltipHOCProps } from '#core/components/cell/components/tooltip/types';
import type { CellRenderer } from '#core/components/cell/types';
import type { TableRow } from '#core/components/table/types/row-types';
import type { TableCellProps } from './types';

/**
 * Creates a tooltip HOC that provides row context and filter actions to custom tooltip content
 *
 * @remarks
 * **Filter behavior:**
 * - If `action="filter"` and the current value already matches `columnFilterValue`, tooltip is hidden
 * - Filter action handler is automatically wired when `action="filter"` and
 *  `setColumnFilterValue` is provided
 *
 * @example
 * ```typescript
 * // Simple filter tooltip
 * const column = {
 *   id: 'status',
 *   label: 'Status',
 *   tooltip: {
 *     content: 'Click to filter by this status',
 *     action: 'filter'
 *   }
 * };

 * // Custom tooltip accessing row data
 * const column = {
 *   id: 'name',
 *   label: 'Name',
 *   tooltip: {
 *     content: ({ row, value }) => (
 *       <div>
 *         <div>Current: {value}</div>
 *         <div>Row has {row.cells.length} columns</div>
 *         <div>Other data: {row.cells.map(c => c.value).join(', ')}</div>
 *       </div>
 *     ),
 *     action: 'custom'
 *   }
 * }
 * ```
 */
export function columnTooltipHOC<T = unknown>(
  Component: CellRenderer<T>,
  row: TableRow<T>,
  columnFilterValue?: TableCellProps['columnFilterValue'],
  setColumnFilterValue?: TableCellProps['setColumnFilterValue']
): CellRenderer<T> {
  return function ColumnTooltipHOC(props: TooltipHOCProps<T>) {
    const { column, value } = props;
    const { action } = column.tooltip;

    // If filter is already applied, render without tooltip
    if (action === 'filter' && isFilterAlreadyApplied(columnFilterValue, value)) {
      return <Component {...props} />;
    }

    const actionHandler = () => {
      if (action === 'filter' && setColumnFilterValue) {
        setColumnFilterValue([value]);
      }
    };

    return (
      <CellTooltipWrapper
        actionHandler={actionHandler}
        content={<ColumnTooltipContentRenderer<T> {...props} row={row} />}
      >
        <Component {...props} />
      </CellTooltipWrapper>
    );
  };
}
