import { CellTooltipContentRenderer } from './cell-tooltip-content-renderer';
import { CellTooltipWrapper } from './cell-tooltip-wrapper';

import type { CellRenderer } from '#core/components/cell/types';
import type { TooltipHOCProps } from './types';

export function cellTooltipHOC<T = unknown>(Component: CellRenderer<T>): CellRenderer<T> {
  return function CellTooltipHOC(props: TooltipHOCProps<T>) {
    return (
      <CellTooltipWrapper {...props} content={<CellTooltipContentRenderer {...props} />}>
        <Component {...props} />
      </CellTooltipWrapper>
    );
  };
}
