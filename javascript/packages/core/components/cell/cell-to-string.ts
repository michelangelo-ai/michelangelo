import { getCellRenderer } from '#core/components/cell/get-cell-renderer';

import type { CellRendererProps } from '#core/components/cell/types';

export function cellToString(props: CellRendererProps<unknown>): string | undefined | null {
  const renderer = getCellRenderer(props.column.type ?? '');
  if (renderer && Object.prototype.hasOwnProperty.call(renderer, 'toString') && renderer.toString)
    return renderer.toString(props);

  const { value } = props;
  if (value === null || value === undefined || value === '') return undefined;

  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
    return String(value);
  }

  return JSON.stringify(value);
}
