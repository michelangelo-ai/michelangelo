import { useStyletron } from 'baseui';
import { StyledTableHead, StyledTableHeadRow } from 'baseui/table-semantic';

import { TableSortIcon } from './components/table-sort-icon/table-sort-icon';
import { StyledSortableTableHeadCell, StyledTableHeadCell } from './styled-components';

import type { TableData } from '#core/components/table/types/data-types';
import type { TableHeaderProps } from './types';

export const TableHeader = <T extends TableData = TableData>({ columns }: TableHeaderProps<T>) => {
  const [css, theme] = useStyletron();
  return (
    <StyledTableHead>
      <StyledTableHeadRow>
        {columns.map((column) =>
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
      </StyledTableHeadRow>
    </StyledTableHead>
  );
};
