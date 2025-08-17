import { styled, withStyle } from 'baseui';
import {
  StyledTableBody as BaseStyledTableBody,
  StyledTableBodyCell as BaseStyledTableBodyCell,
  StyledTableBodyRow as BaseStyledTableBodyRow,
} from 'baseui/table-semantic';

export const TableContainer = styled('div', ({ $theme }) => ({
  overflow: 'auto',
  position: 'relative',
  display: 'flex',
  flexDirection: 'column',
  gap: $theme.sizing.scale400,
}));

export const StyledTableBody = withStyle(BaseStyledTableBody, ({ $theme }) => ({
  backgroundColor: $theme.colors.tableHeadBackgroundColor,
}));

export const StyledTableBodyRow = BaseStyledTableBodyRow;

/**
 * By default, all cells within the first column are bold
 */
export const StyledTableBodyCell = withStyle<
  typeof BaseStyledTableBodyCell,
  { $columnNumber?: number }
>(BaseStyledTableBodyCell, ({ $columnNumber }) => ({
  fontWeight: $columnNumber === 0 ? 500 : 'normal',
  verticalAlign: 'middle',
}));
