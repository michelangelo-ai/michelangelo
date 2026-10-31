import { TaskStateIcon } from '../task-state-icon';
import {
  TimetableAxis,
  TimetableAxisLabel,
  TimetableColumnLabel,
  TimetableDuration,
  TimetableDurationLabel,
  TimetableGridLine,
  TimetableNote,
  TimetableRowGrid,
  TimetableTaskCell,
  TimetableTaskName,
  TimetableTrack,
} from './styled-components';
import { TaskTimetableBar } from './task-timetable-bar';
import { buildTimetable } from './utils/build-timetable';
import { formatDuration } from './utils/format-duration';
import { toPercent } from './utils/to-percent';

import type { KeyboardEvent } from 'react';
import type { TaskTimetableProps } from './types';

const NO_DURATION_LABEL = '—';

/**
 * Gantt-style alternative to the task flow: every task in the tree, subtasks
 * included, becomes a row whose bar spans the time the task took on a timeline
 * shared by all rows. Rows are clickable and scroll to the matching task section.
 */
export function TaskTimetable<TTaskRecord extends object = object>({
  taskList,
  onTaskClick,
}: TaskTimetableProps<TTaskRecord>) {
  const { rows, totalSeconds, ticks, hasTiming } = buildTimetable(taskList);

  return (
    <div>
      {hasTiming ? (
        <TimetableRowGrid>
          <TimetableColumnLabel>Task</TimetableColumnLabel>
          <TimetableAxis>
            {ticks.map((tick) => (
              <TimetableAxisLabel
                key={tick.offsetSeconds}
                $offsetPercent={toPercent(tick.offsetSeconds, totalSeconds)}
              >
                {tick.label}
              </TimetableAxisLabel>
            ))}
          </TimetableAxis>
          <TimetableDurationLabel>Duration</TimetableDurationLabel>
        </TimetableRowGrid>
      ) : (
        <TimetableNote>No timing information is available for these tasks.</TimetableNote>
      )}

      {rows.map((row, index) => {
        const handleActivate = () => onTaskClick(row.task, row.ancestors);

        return (
          <TimetableRowGrid
            key={index}
            $interactive
            role="button"
            tabIndex={0}
            onClick={handleActivate}
            onKeyDown={(event: KeyboardEvent) => {
              if (event.key !== 'Enter' && event.key !== ' ') return;
              event.preventDefault();
              handleActivate();
            }}
          >
            <TimetableTaskCell $depth={row.depth}>
              <TaskStateIcon state={row.task.state} />
              <TimetableTaskName>{row.task.name}</TimetableTaskName>
            </TimetableTaskCell>
            <TimetableTrack>
              {ticks.map((tick) => (
                <TimetableGridLine
                  key={tick.offsetSeconds}
                  $offsetPercent={toPercent(tick.offsetSeconds, totalSeconds)}
                />
              ))}
              <TaskTimetableBar row={row} totalSeconds={totalSeconds} />
            </TimetableTrack>
            <TimetableDuration>
              {row.durationSeconds != null
                ? formatDuration(row.durationSeconds)
                : NO_DURATION_LABEL}
            </TimetableDuration>
          </TimetableRowGrid>
        );
      })}
    </div>
  );
}
