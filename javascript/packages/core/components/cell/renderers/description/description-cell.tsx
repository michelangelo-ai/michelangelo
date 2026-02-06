import { useStyletron } from 'baseui';

import { DescriptionText } from '#core/components/description-text';
import { TruncatedText } from '#core/components/truncated-text/truncated-text';
import { DescriptionHierarchy } from './constants';

import type { CellRendererProps } from '#core/components/cell/types';
import type { DescriptionCellConfig } from './types';

/**
 * Cell renderer for descriptive text with optional hierarchy styling.
 *
 * Displays text in secondary color (or primary if PRIMARY hierarchy is specified) using
 * small paragraph typography. Includes automatic truncation for long text.
 *
 * @param props.value - Description text to display
 * @param props.column - Column configuration with optional hierarchy
 *   - column.hierarchy: PRIMARY for primary color, otherwise secondary color
 *
 * @example
 * ```tsx
 * // Secondary description (default)
 * { id: 'notes', label: 'Notes', type: CellType.DESCRIPTION }
 * // Renders in gray color
 *
 * // Primary hierarchy
 * {
 *   id: 'subtitle',
 *   type: CellType.DESCRIPTION,
 *   hierarchy: DescriptionHierarchy.PRIMARY
 * }
 * // Renders in primary text color
 * ```
 */
export const DescriptionCell = ({
  column,
  value,
}: CellRendererProps<string, DescriptionCellConfig>) => {
  const [, theme] = useStyletron();
  return (
    <DescriptionText
      {...(column.hierarchy === DescriptionHierarchy.PRIMARY
        ? { $styleOverrides: { color: theme.colors.contentPrimary } }
        : {})}
    >
      <TruncatedText>{value}</TruncatedText>
    </DescriptionText>
  );
};
