import { styled, withStyle } from 'baseui';
import { StyledTableBody, StyledTableBodyCell } from 'baseui/table-semantic';

// Container for the table with scroll behavior - used by main Table component
export const TableContainer = styled('div', {
  overflow: 'auto',
  position: 'relative',
});

// Table body with themed background - used by TableLoadingState
export const Body = withStyle(StyledTableBody, ({ $theme }) => ({
  backgroundColor: $theme.colors.tableHeadBackgroundColor,
}));

// Table row with hover effect - used by TableLoadingState
export const Row = styled<'tr', {}>('tr', ({ $theme }) => ({
  ':hover': {
    backgroundColor: $theme.colors.tableStripedBackground,
  },
}));

// Table cell container - used by TableLoadingState
export const CellContainer = withStyle<typeof StyledTableBodyCell, { $columnNumber?: number }>(
  StyledTableBodyCell,
  ({ $columnNumber }) => ({
    fontWeight: $columnNumber === 0 ? 500 : 'normal',
    verticalAlign: 'middle',
  })
);
