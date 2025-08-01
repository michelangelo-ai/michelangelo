import { withStyle } from 'baseui';
import { StyledTableHeadCell as BaseStyledTableHeadCell } from 'baseui/table-semantic';

// Unset z-index to prevent interference with popovers and overlays
export const StyledTableHeadCell = withStyle(BaseStyledTableHeadCell, {
  zIndex: 'unset',
});
