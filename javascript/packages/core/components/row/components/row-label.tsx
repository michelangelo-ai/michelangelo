import { useStyletron } from 'baseui';

import type { RowProps } from '#core/components/row/types';

export function RowLabel(props: { label: RowProps['items'][number]['label'] }) {
  const [css, theme] = useStyletron();

  return (
    <div
      className={css({
        ...theme.typography.LabelSmall,
        color: theme.colors.contentTertiary,
        marginBottom: theme.sizing.scale300,
      })}
    >
      {props.label}
    </div>
  );
}
