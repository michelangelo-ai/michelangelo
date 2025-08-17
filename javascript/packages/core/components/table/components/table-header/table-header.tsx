import { useStyletron } from 'baseui';
import { StyledTableHead, StyledTableHeadRow } from 'baseui/table-semantic';

import { TableSortIcon } from './components/table-sort-icon/table-sort-icon';
import { StyledSortableTableHeadCell, StyledTableHeadCell } from './styled-components';

import type { TableData } from '#core/components/table/types/data-types';
import type { TableHeaderProps } from './types';

export const TableHeader = <T extends TableData = TableData>({ headers }: TableHeaderProps<T>) => {
  const [css, theme] = useStyletron();
  return (
    <StyledTableHead>
      <StyledTableHeadRow>
        {headers.map((header) =>
          header.canSort ? (
            <StyledSortableTableHeadCell
              key={header.id}
              $isFocusVisible={false}
              onClick={header.onToggleSort}
              role="columnheader"
            >
              <div
                className={css({
                  display: 'flex',
                  alignItems: 'center',
                  gap: theme.sizing.scale300,
                })}
              >
                <div>{header.content}</div>
                <TableSortIcon column={{ getIsSorted: () => header.sortDirection ?? false }} />
              </div>
            </StyledSortableTableHeadCell>
          ) : (
            <StyledTableHeadCell key={header.id} role="columnheader">
              {header.content}
            </StyledTableHeadCell>
          )
        )}
      </StyledTableHeadRow>
    </StyledTableHead>
  );
};
