import { useStyletron } from 'baseui';
import { StyledTableHead, StyledTableHeadRow } from 'baseui/table-semantic';

import { getSelectionColumnCellStyles } from '../table-selection-column/styled-components';
import { TableSelectionColumn } from '../table-selection-column/table-selection-column';
import { withStickySides } from '../with-sticky-sides/with-sticky-sides';
import { TableColumnConfigurationButton } from './components/table-column-configuration-button/table-column-configuration-button';
import { TableSortIcon } from './components/table-sort-icon/table-sort-icon';
import { StyledSortableTableHeadCell, StyledTableHeadCell } from './styled-components';

import type { TableData } from '#core/components/table/types/data-types';
import type { TableHeaderProps } from './types';

const StickySidesTableHeadRow = withStickySides(StyledTableHeadRow);

export const TableHeader = <T extends TableData = TableData>({
  columns,
  setColumnOrder,
  setColumnVisibility,
  enableRowSelection,
  isSelected,
  onToggleSelection,
  enableStickySides,
  scrollRatio,
}: TableHeaderProps<T>) => {
  const [css, theme] = useStyletron();

  return (
    <StyledTableHead>
      <StickySidesTableHeadRow
        enableStickySides={enableStickySides}
        enableRowSelection={enableRowSelection}
        lastColumnIndex={columns.filter((column) => column.isVisible).length + 1}
        scrollRatio={scrollRatio}
        role="header"
      >
        {enableRowSelection && (
          <StyledTableHeadCell
            role="columnheader"
            className={css(getSelectionColumnCellStyles(theme))}
          >
            <TableSelectionColumn
              canSelect={enableRowSelection}
              isSelected={isSelected}
              onToggleSelection={onToggleSelection}
            />
          </StyledTableHeadCell>
        )}

        {columns
          .filter((column) => column.isVisible)
          .map((column) =>
            column.canSort ? (
              <StyledSortableTableHeadCell
                key={column.id}
                $isFocusVisible={false}
                onClick={column.onToggleSort}
                role="columnheader"
              >
                <div
                  className={css({
                    display: 'flex',
                    alignItems: 'center',
                    gap: theme.sizing.scale300,
                  })}
                >
                  <div>{column.label}</div>
                  <TableSortIcon column={{ getIsSorted: () => column.sortDirection ?? false }} />
                </div>
              </StyledSortableTableHeadCell>
            ) : (
              <StyledTableHeadCell key={column.id} role="columnheader">
                {column.label}
              </StyledTableHeadCell>
            )
          )}

        {setColumnOrder && setColumnVisibility && (
          <StyledTableHeadCell role="columnheader">
            <div className={css({ display: 'flex', justifyContent: 'center' })}>
              <TableColumnConfigurationButton
                columns={columns}
                setColumnOrder={setColumnOrder}
                setColumnVisibility={setColumnVisibility}
              />
            </div>
          </StyledTableHeadCell>
        )}
      </StickySidesTableHeadRow>
    </StyledTableHead>
  );
};
