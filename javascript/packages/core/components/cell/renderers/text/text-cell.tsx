import { useCellToString } from '#core/components/cell/use-cell-to-string';
import { TruncatedText } from '#core/components/truncated-text/truncated-text';

import type { CellRenderer } from '#core/components/cell/types';

/**
 * Default cell renderer for text values with automatic truncation.
 *
 * Displays text content with ellipsis overflow handling and tooltip for long text.
 * Shows em dash (—) for null/undefined/empty values.
 *
 * @param props - Cell renderer props with string value
 *
 * @example
 * ```tsx
 * // Automatically used for string columns
 * { id: 'description', label: 'Description' }
 * // Renders: "Long text content..." (with tooltip on hover)
 * ```
 */
export const TextCell: CellRenderer<string> = (props) => {
  const cellToString = useCellToString();
  return <TruncatedText>{cellToString(props) ?? '\u2014'}</TruncatedText>;
};
