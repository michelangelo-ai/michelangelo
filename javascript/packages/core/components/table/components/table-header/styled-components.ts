import { withStyle } from 'baseui';
import {
  StyledTableHeadCell as BaseStyledTableHeadCell,
  StyledTableHeadCellSortable as BaseStyledTableHeadCellSortable,
} from 'baseui/table-semantic';

export const StyledTableHeadCell = withStyle(BaseStyledTableHeadCell, ({ $theme }) => ({
  zIndex: 'unset', // Unset z-index to prevent interference with popovers and overlays
  position: 'inherit', // Position inherit to allow for sticky columns
  ...$theme.typography.LabelSmall,
}));

export const StyledSortableTableHeadCell = withStyle(
  BaseStyledTableHeadCellSortable,
  ({ $theme }) => ({
    zIndex: 'unset', // Unset z-index to prevent interference with popovers and overlays
    position: 'inherit', // Position inherit to allow for sticky columns
    ...$theme.typography.LabelSmall,
  })
);

// BaseWeb Button comes with its own padding to support good hoverable areas. When combined
// with `StyledTableHeadCell`, this padding creates too tall of a header row.
export const StyledTableConfigurationButtonHeadCell = withStyle(StyledTableHeadCell, {
  padding: 0,
});
