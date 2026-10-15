import { TimetableBar, TimetableMarker } from './styled-components';
import { toPercent } from './utils/to-percent';

import type { TaskTimetableBarProps } from './types';

export function TaskTimetableBar<TTaskRecord extends object>({
  row,
  totalSeconds,
}: TaskTimetableBarProps<TTaskRecord>) {
  if (row.startOffset === undefined || row.durationSeconds === undefined) return null;

  const offsetPercent = toPercent(row.startOffset, totalSeconds);

  // A task known by a single timestamp has no span to draw, so it reads as a point in time
  if (row.durationSeconds === 0) {
    return <TimetableMarker $state={row.task.state} $offsetPercent={offsetPercent} />;
  }

  return (
    <TimetableBar
      $state={row.task.state}
      $offsetPercent={offsetPercent}
      $widthPercent={Math.min(toPercent(row.durationSeconds, totalSeconds), 100 - offsetPercent)}
      $ongoing={row.ongoing}
    />
  );
}
