import { withStyle } from 'baseui';
import { StyledTableBodyCell as BaseStyledTableBodyCell } from 'baseui/table-semantic';

export const StyledTableBodyCell = withStyle<
  typeof BaseStyledTableBodyCell,
  { $columnNumber?: number }
>(BaseStyledTableBodyCell, ({ $columnNumber }) => ({
  fontWeight: $columnNumber === 0 ? 500 : 'normal',
  verticalAlign: 'middle',
}));
