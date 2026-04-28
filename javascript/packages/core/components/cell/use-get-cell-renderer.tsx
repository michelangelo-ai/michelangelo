import isURL from 'validator/lib/isURL';

import { CELL_RENDERERS } from '#core/components/cell/constants';
import { TextCell } from '#core/components/cell/renderers/text/text-cell';
import { Link } from '#core/components/link/link';
import { useCellProvider } from '#core/providers/cell-provider/use-cell-provider';
import { CellType } from './constants';

import type { CellRenderer, CellRendererProps } from '#core/components/cell/types';

/**
 * Returns a function that resolves the appropriate cell renderer based on column
 * configuration and value type.
 *
 * The renderer resolution follows this priority order:
 * 1. Custom renderer from column.Cell if provided
 * 2. Renderer from CellProvider context if registered for the column type
 * 3. Built-in renderer from CELL_RENDERERS if type matches
 * 4. Auto-detected link renderer for URL values
 * 5. Default TextCell renderer as fallback
 *
 * @returns Function that takes CellRendererProps and returns the appropriate CellRenderer.
 *   The returned renderer will handle rendering the cell value and provide a toString method.
 *
 * @example
 * ```typescript
 * const getCellRenderer = useGetCellRenderer();
 *
 * // Get renderer for a specific cell
 * const renderer = getCellRenderer({
 *   column: { type: CellType.DATE },
 *   value: '2024-01-15T10:30:00Z'
 * });
 * // Returns DateCell renderer
 *
 * // Custom renderer from column config
 * const renderer = getCellRenderer({
 *   column: { Cell: MyCustomCell },
 *   value: data
 * });
 * // Returns MyCustomCell
 *
 * // Auto-detected URL
 * const renderer = getCellRenderer({
 *   column: {},
 *   value: 'https://example.com'
 * });
 * // Returns auto-generated Link renderer
 *
 * // Fallback for unknown types
 * const renderer = getCellRenderer({
 *   column: {},
 *   value: 'plain text'
 * });
 * // Returns TextCell
 * ```
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

    if (typeof value === 'string' && isURL(value, { require_protocol: true, require_tld: false })) {
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
