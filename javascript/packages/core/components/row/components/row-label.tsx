import { useStyletron } from 'baseui';

import { HelpTooltip } from '#core/components/help-tooltip';

import type { RowProps } from '#core/components/row/types';

export function RowLabel(props: {
  label: RowProps['items'][number]['label'];
  description?: RowProps['items'][number]['description'];
}) {
  const [css, theme] = useStyletron();

  return (
    <div
      className={css({
        alignItems: 'center',
        ...theme.typography.LabelSmall,
        color: theme.colors.contentTertiary,
        display: 'flex',
        gap: theme.sizing.scale200,
        marginBottom: theme.sizing.scale300,
      })}
    >
      {props.label}
      {props.description ? <HelpTooltip text={props.description} /> : null}
    </div>
  );
}
