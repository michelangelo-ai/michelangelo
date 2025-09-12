import { Markdown } from '#core/components/markdown/markdown';

import type { CellTooltipContentRenderer as _CellTooltipContentRenderer } from '#core/components/cell/components/tooltip/cell-tooltip-content-renderer';
import type { ColumnTooltipContentRendererProps } from './types';

/**
 * Table-specific tooltip content renderer that provides row data access to custom
 * tooltip components.
 *
 * @remarks
 * This renderer extends {@link _CellTooltipContentRenderer} from the base cell system by adding
 * table row context. Use the base renderer for non-table contexts where row data isn't needed.
 */
export function ColumnTooltipContentRenderer<T = unknown>(
  props: ColumnTooltipContentRendererProps<T>
) {
  if (typeof props.column.tooltip.content === 'function') {
    const Component = props.column.tooltip.content;
    return <Component {...props} />;
  }

  return <Markdown>{props.column.tooltip.content}</Markdown>;
}
