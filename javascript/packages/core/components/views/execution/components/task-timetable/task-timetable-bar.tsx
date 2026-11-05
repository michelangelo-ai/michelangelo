import { TimetableBar, TimetableMarker } from './styled-components';
import { toPercent } from './utils/to-percent';

import type { TaskTimetableBarProps } from './types';

export function TaskTimetableBar<TTaskRecord extends object>({
  row,
  totalSeconds,
}: TaskTimetableBarProps<TTaskRecord>) {
  if (row.startOffset === undefined || row.durationSeconds === undefined) return null;

  const offsetPercent = toPercent(row.startOffset, totalSeconds);
  const widthPercent = Math.min(
    toPercent(row.durationSeconds, totalSeconds),
    100 - offsetPercent
  );

  // Point-in-time: no span (or shorter than 0.5s, which formats as "0s"). A min-width
  // bar would sit with its left edge on the tick and look like an off-center border
  // around what reads as a circle — draw a centered marker instead.
  if (Math.round(row.durationSeconds) === 0 || widthPercent === 0) {
    return <TimetableMarker $state={row.task.state} $offsetPercent={offsetPercent} />;
  }

  return (
    <TimetableBar
      $state={row.task.state}
      $offsetPercent={offsetPercent}
      $widthPercent={widthPercent}
      $ongoing={row.ongoing}
    />
  );
}
