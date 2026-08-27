import { useStyletron } from 'baseui';

import { TAG_BEHAVIOR, TAG_HIERARCHY, TAG_SIZE } from '#core/components/tag/constants';
import { Tag } from '#core/components/tag/tag';
import { STEP_STATE_COLOR_MAP, STEP_STATE_TEXT_MAP } from '#core/config/entities/run/shared';
import { formatElapsedSeconds, timestampToString } from '#core/utils/time-utils';

import type { PipelineRunStepInfo } from '#core/config/entities/run/types';

/**
 * One row in the resume step picker: task name, when it ran, how long it took,
 * and how it ended.
 *
 * Steps that never started show an em dash rather than a zeroed timestamp — a
 * skipped step has no meaningful start, end, or duration.
 */
export const ResumeStepOption = ({ step }: { step?: PipelineRunStepInfo }) => {
  const [css, theme] = useStyletron();

  if (!step) return null;

  const start = timestampToString(step.startTime?.seconds);
  const end = timestampToString(step.endTime?.seconds);
  const duration = formatElapsedSeconds(step.startTime?.seconds, step.endTime?.seconds);
  const state = step.state ?? 0;

  const metadataStyles = css({ ...theme.typography.ParagraphXSmall });

  return (
    <div
      className={css({
        display: 'grid',
        gridTemplateColumns: '1.5fr 1fr 1fr 0.5fr auto',
        alignItems: 'center',
        gap: theme.sizing.scale400,
        width: '100%',
      })}
    >
      <span>{step.displayName}</span>
      <span className={metadataStyles}>{start ?? '—'}</span>
      <span className={metadataStyles}>{end ?? '—'}</span>
      <span className={metadataStyles}>{duration ?? '—'}</span>
      <Tag
        size={TAG_SIZE.xSmall}
        hierarchy={TAG_HIERARCHY.secondary}
        behavior={TAG_BEHAVIOR.display}
        closeable={false}
        color={STEP_STATE_COLOR_MAP[state] ?? 'gray'}
      >
        {STEP_STATE_TEXT_MAP[state] ?? 'Unknown'}
      </Tag>
    </div>
  );
};
