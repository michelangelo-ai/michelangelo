import { useStyletron } from 'baseui';

import { Icon } from '#core/components/icon/icon';
import { IconKind } from '#core/components/icon/types';

import type { CellRendererProps } from '#core/components/cell/types';

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
      <Icon name="circleCheckFilled" kind={IconKind.ACCENT} size={14} />
      {BooleanCell.toString({ column, value })}
    </div>
  );
};

BooleanCell.toString = (props: Pick<CellRendererProps<boolean>, 'column' | 'value'>) => {
  if (!props.value) return '';

  return props.column.label ?? 'True';
};
