import { useGetCellRenderer } from '#core/components/cell/use-get-cell-renderer';

import type { CellRendererProps } from '#core/components/cell/types';

/**
 * @returns A function that returns a string representation of a cell's value.
 */
export function useCellToString(): (
  props: CellRendererProps<unknown>
) => string | undefined | null {
  const getCellRenderer = useGetCellRenderer();

  return (props: CellRendererProps<unknown>) => {
    const renderer = getCellRenderer(props);
    if (renderer && Object.prototype.hasOwnProperty.call(renderer, 'toString') && renderer.toString)
      return renderer.toString(props);

    const { value } = props;
    if (value === null || value === undefined || value === '') return undefined;

    if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
      return String(value);
    }

    return JSON.stringify(value);
  };
}
