import { styled, withStyle } from 'baseui';
import {
  StyledTable as BaseStyledTable,
  StyledTableBody as BaseStyledTableBody,
  StyledTableBodyCell as BaseStyledTableBodyCell,
  StyledTableBodyRow as BaseStyledTableBodyRow,
} from 'baseui/table-semantic';

export const StyledTable = BaseStyledTable;

export const StyledTableBody = withStyle(BaseStyledTableBody, ({ $theme }) => ({
  backgroundColor: $theme.colors.tableHeadBackgroundColor,
}));

export const StyledTableBodyRow = BaseStyledTableBodyRow;

/**
 * By default, all cells within the first column are bold
 */
export const StyledTableBodyCell = withStyle<
  typeof BaseStyledTableBodyCell,
  { $columnNumber?: number; $enableRowSelection?: boolean }
>(BaseStyledTableBodyCell, ({ $columnNumber, $enableRowSelection }) => {
  const firstDataColumnIndex = $enableRowSelection ? 1 : 0;

  return {
    fontWeight: $columnNumber === firstDataColumnIndex ? 500 : 'normal',
    verticalAlign: 'middle',
  };
});

export const StyledActionCell = styled('div', {
  display: 'flex',
  justifyContent: 'center',
});
