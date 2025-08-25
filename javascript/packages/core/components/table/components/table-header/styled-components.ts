import { withStyle } from 'baseui';
import {
  StyledTableHeadCell as BaseStyledTableHeadCell,
  StyledTableHeadCellSortable as BaseStyledTableHeadCellSortable,
} from 'baseui/table-semantic';

export const StyledTableHeadCell = withStyle(BaseStyledTableHeadCell, () => ({
  zIndex: 'unset', // Unset z-index to prevent interference with popovers and overlays
  position: 'inherit', // Position inherit to allow for sticky columns
}));

export const StyledSortableTableHeadCell = withStyle(BaseStyledTableHeadCellSortable, () => ({
  zIndex: 'unset', // Unset z-index to prevent interference with popovers and overlays
  position: 'inherit', // Position inherit to allow for sticky columns
}));
