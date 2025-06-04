import type { ReactNode } from 'react';
import type { CellRendererProps, CellTooltip, SharedCell } from '#core/components/cell/types';

/**
 * Props for the Tooltip Higher-Order Component (HOC).
 *
 * @template T - The type of the data being rendered in the cell.
 * @extends CellRendererProps<T> - Extends the properties of the cell renderer.
 *
 * @property column - A column configuration with tooltip specification
 */
export type TooltipHOCProps<T = unknown> = CellRendererProps<T> & {
  column: Omit<SharedCell, 'tooltip'> & { tooltip: CellTooltip };
};

/**
 * Props for the TooltipWrapper component.
 *
 * @remarks
 * The TooltipWrapper component expects an actionHandler that can be provided
 * directly to an onClick property. This actionHandler is derived from
 * the {@link Column} tooltip.action property
 *
 * @property {TooltipActionHandler} [actionHandler] - Optional handler for tooltip actions.
 * @property {ReactNode} children - The tooltip anchor
 * @property {ReactNode} content - The content to be displayed in the tooltip.
 */
export interface TooltipWrapperProps {
  actionHandler?: TooltipActionHandler;
  children: ReactNode;
  content: ReactNode;
}

export type TooltipActionHandler = () => void;
