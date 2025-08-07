import { useStyletron } from 'baseui';
import { LabelSmall, ParagraphSmall } from 'baseui/typography';

import { Markdown } from '#core/components/markdown/markdown';

import type { TaskBodyTextAreaProps } from './types';

export function TaskBodyTextarea(props: TaskBodyTextAreaProps) {
  const [css, theme] = useStyletron();
  const { label, value, markdown = true, error = false } = props;

  if (!value) return null;

  const trimmedText = value.substring(0, 10000);

  return (
    <div
      className={css({
        backgroundColor: error
          ? theme.colors.backgroundNegativeLight
          : theme.colors.backgroundSecondary,
        borderRadius: theme.borders.radius300,
        minHeight: theme.sizing.scale1600,
        overflow: 'auto',
        padding: theme.sizing.scale600,
        resize: 'vertical',
      })}
    >
      <LabelSmall id="textarea-title">{label}</LabelSmall>
      <ParagraphSmall
        aria-labelledby="textarea-title"
        className={css({ marginBottom: 0, marginTop: theme.sizing.scale100 })}
      >
        {markdown ? <Markdown>{trimmedText}</Markdown> : trimmedText}
      </ParagraphSmall>
    </div>
  );
}
