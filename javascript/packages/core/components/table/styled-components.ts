import { withStyle } from 'baseui';
import {
  StyledTableBody as BaseStyledTableBody,
  StyledTableBodyCell as BaseStyledTableBodyCell,
  StyledTableBodyRow as BaseStyledTableBodyRow,
} from 'baseui/table-semantic';

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
