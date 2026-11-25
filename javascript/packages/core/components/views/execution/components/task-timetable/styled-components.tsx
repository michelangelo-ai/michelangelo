import { styled } from 'baseui';

import { STATE_TO_STYLE_MAP } from '#core/components/views/execution/constants';
import { ELLIPSIS_STYLES } from '#core/styles/constants';

import type { TaskState } from '#core/components/views/execution/types';

/** Solid fill for timetable bars/markers — prefer the style map's text color, else its border. */
function barColorName(state: TaskState): string {
  const style = STATE_TO_STYLE_MAP[state];
  return style.colorName ?? style.borderColorName;
}

/** Shared column layout so the axis header stays aligned with every task row. */
const ROW_TEMPLATE = 'minmax(96px, 200px) 1fr 76px';

const BAR_HEIGHT = '10px';

export const TimetableRowGrid = styled<'div', { $interactive?: boolean }>(
  'div',
  ({ $theme, $interactive }) => ({
    display: 'grid',
    gridTemplateColumns: ROW_TEMPLATE,
    alignItems: 'center',
    gap: $theme.sizing.scale300,
    padding: `${$theme.sizing.scale100} ${$theme.sizing.scale200}`,
    borderRadius: $theme.borders.radius200,
    ...($interactive
      ? {
          cursor: 'pointer',
          ':hover': { backgroundColor: $theme.colors.backgroundSecondary },
          ':focus-visible': {
            outline: `2px solid ${$theme.colors.borderAccent}`,
            outlineOffset: '-2px',
          },
        }
      : {}),
  })
);

export const TimetableColumnLabel = styled('div', ({ $theme }) => ({
  ...$theme.typography.LabelXSmall,
  color: $theme.colors.contentTertiary,
}));

export const TimetableDurationLabel = styled('div', ({ $theme }) => ({
  ...$theme.typography.LabelXSmall,
  color: $theme.colors.contentTertiary,
  textAlign: 'right',
}));

export const TimetableTaskCell = styled<'div', { $depth: number }>('div', ({ $theme, $depth }) => ({
  display: 'flex',
  alignItems: 'center',
  gap: $theme.sizing.scale300,
  minWidth: 0,
  paddingLeft: `calc(${$theme.sizing.scale600} * ${$depth})`,
  ...$theme.typography.LabelSmall,
}));

export const TimetableTaskName = styled('div', () => ELLIPSIS_STYLES);

export const TimetableTrack = styled('div', ({ $theme }) => ({
  position: 'relative',
  minWidth: 0,
  height: $theme.sizing.scale750,
}));

export const TimetableAxis = styled('div', ({ $theme }) => ({
  position: 'relative',
  minWidth: 0,
  height: $theme.sizing.scale600,
  ...$theme.typography.LabelXSmall,
  color: $theme.colors.contentTertiary,
}));

export const TimetableAxisLabel = styled<'div', { $offsetPercent: number }>(
  'div',
  ({ $offsetPercent }) => ({
    position: 'absolute',
    top: 0,
    left: `${$offsetPercent}%`,
    // Center on the tick so labels line up with grid lines and zero-duration markers.
    transform: 'translateX(-50%)',
    whiteSpace: 'nowrap',
  })
);

export const TimetableGridLine = styled<'div', { $offsetPercent: number }>(
  'div',
  ({ $theme, $offsetPercent }) => ({
    position: 'absolute',
    top: 0,
    bottom: 0,
    left: `${$offsetPercent}%`,
    // Center the 1px stroke on the tick — matches TimetableMarker, which is also
    // centered on the timestamp. Without this, a 0s marker looks offset from the start line.
    transform: 'translateX(-50%)',
    width: '1px',
    backgroundColor: $theme.colors.contentInverseTertiary,
  })
);

/**
 * Horizontal bar spanning the time a task took, colored by execution state.
 * Running tasks whose end time is unknown are striped to signal an open range.
 */
export const TimetableBar = styled<
  'div',
  { $state: TaskState; $offsetPercent: number; $widthPercent: number; $ongoing: boolean }
>('div', ({ $theme, $state, $offsetPercent, $widthPercent, $ongoing }) => ({
  position: 'absolute',
  top: '50%',
  left: `${$offsetPercent}%`,
  transform: 'translateY(-50%)',
  width: `${$widthPercent}%`,
  minWidth: '6px',
  height: BAR_HEIGHT,
  borderRadius: BAR_HEIGHT,
  backgroundColor: $theme.colors[barColorName($state)],
  ...($ongoing
    ? {
        backgroundImage:
          'repeating-linear-gradient(135deg, rgba(255, 255, 255, 0.45) 0 4px, transparent 4px 8px)',
      }
    : {}),
}));

/** Point marker for tasks known by a single timestamp, which have no span to draw. */
export const TimetableMarker = styled<'div', { $state: TaskState; $offsetPercent: number }>(
  'div',
  ({ $theme, $state, $offsetPercent }) => ({
    position: 'absolute',
    top: '50%',
    left: `${$offsetPercent}%`,
    transform: 'translate(-50%, -50%)',
    width: BAR_HEIGHT,
    height: BAR_HEIGHT,
    borderRadius: '50%',
    backgroundColor: $theme.colors[barColorName($state)],
  })
);

export const TimetableDuration = styled('div', ({ $theme }) => ({
  ...$theme.typography.ParagraphXSmall,
  color: $theme.colors.contentSecondary,
  textAlign: 'right',
  fontVariantNumeric: 'tabular-nums',
}));
