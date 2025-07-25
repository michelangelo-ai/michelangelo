import { styled, withStyle } from 'baseui';
import { StyledTableBody as BaseStyledTableBody } from 'baseui/table-semantic';

export const TableContainer = styled('div', {
  overflow: 'auto',
  position: 'relative',
});

export const StyledTableBody = withStyle(BaseStyledTableBody, ({ $theme }) => ({
  backgroundColor: $theme.colors.tableHeadBackgroundColor,
}));
