import isURL from 'validator/lib/isURL';

import { CELL_RENDERERS } from '#core/components/cell/constants';
import { TextCell } from '#core/components/cell/renderers/text/text-cell';
import { CellRenderer, CellRendererProps } from '#core/components/cell/types';
import { Link } from '#core/components/link/link';
import { CellType } from './constants';

export function getCellRenderer(args: CellRendererProps<unknown>): CellRenderer<unknown> {
  const { column, value } = args;

  const { Cell } = column;
  if (Cell) {
    return Cell;
  }

  const columnType = getType(args);
  if (columnType && columnType in CELL_RENDERERS) {
    return CELL_RENDERERS[columnType];
  }

  if (typeof value === 'string' && isURL(value)) {
    const LinkRenderer = () => <Link href={value}>Click here</Link>;
    LinkRenderer.displayName = 'LinkRenderer';
    return LinkRenderer;
  }

  return TextCell;
}

function getType(args: CellRendererProps): string | undefined {
  const { column } = args;

  if ('url' in column) return CellType.LINK;

  return column.type;
}
