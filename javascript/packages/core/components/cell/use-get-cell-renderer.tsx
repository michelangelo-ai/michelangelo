import isURL from 'validator/lib/isURL';

import { CELL_RENDERERS } from '#core/components/cell/constants';
import { TextCell } from '#core/components/cell/renderers/text/text-cell';
import { CellRenderer, CellRendererProps } from '#core/components/cell/types';
import { Link } from '#core/components/link/link';
import { useCellProvider } from '#core/providers/cell-provider/use-cell-provider';
import { CellType } from './constants';

/**
 * @returns A function that returns a cell renderer for a given column.
 */
export function useGetCellRenderer(): (args: CellRendererProps<unknown>) => CellRenderer<unknown> {
  const cellContext = useCellProvider();

  return (args: CellRendererProps<unknown>) => {
    const { column, value } = args;

    const { Cell } = column;
    if (Cell) {
      return Cell;
    }

    const columnType = getType(args);

    if (columnType && cellContext?.renderers[columnType]) {
      return cellContext.renderers[columnType];
    }

    if (columnType && columnType in CELL_RENDERERS) {
      return CELL_RENDERERS[columnType];
    }

    if (typeof value === 'string' && isURL(value, { require_protocol: true })) {
      const LinkRenderer = () => <Link href={value}>Click here</Link>;
      LinkRenderer.displayName = 'LinkRenderer';
      return LinkRenderer;
    }

    return TextCell;
  };
}

function getType(args: CellRendererProps): string | undefined {
  const { column } = args;

  if ('items' in column) return CellType.MULTI;
  if ('url' in column) return CellType.LINK;

  return column.type;
}
