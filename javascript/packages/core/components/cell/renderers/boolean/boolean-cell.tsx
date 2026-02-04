import { useStyletron } from 'baseui';

import { Icon } from '#core/components/icon/icon';
import { IconKind } from '#core/components/icon/types';

import type { CellRendererProps } from '#core/components/cell/types';

/**
 * Cell renderer for boolean values that displays a checkmark icon with optional label.
 *
 * Shows a filled check icon with accent color when value is true. Renders nothing when false.
 * The label text defaults to column.label or "True" when value is true.
 *
 * @param props.value - Boolean value to render
 * @param props.column - Column configuration (optional label)
 *
 * @example
 * ```tsx
 * // In table column definition
 * {
 *   id: 'isActive',
 *   label: 'Active',
 *   type: CellType.BOOLEAN
 * }
 * // Renders: ✓ Active (when true)
 * // Renders: (nothing when false)
 * ```
 */
export const BooleanCell = ({ value, column }: CellRendererProps<boolean>) => {
  const [css, theme] = useStyletron();

  if (!value) return null;

  return (
    <div
      className={css({
        alignItems: 'center',
        display: 'flex',
        gap: '6px',
        color: theme.colors.contentAccent,
        ...theme.typography.ParagraphSmall,
      })}
    >
      <Icon name="circleCheckFilled" kind={IconKind.ACCENT} size={theme.sizing.scale550} />
      {BooleanCell.toString({ column, value })}
    </div>
  );
};

BooleanCell.toString = (props: Pick<CellRendererProps<boolean>, 'column' | 'value'>) => {
  if (!props.value) return '';

  return props.column.label ?? 'True';
};
