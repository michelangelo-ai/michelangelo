import { TASK_STATE } from '../../../constants';
import { flattenTaskList } from './flatten-task-list';
import { DAY, formatDuration, HOUR, MINUTE } from './format-duration';
import { resolveTaskTiming } from './resolve-task-timing';

import type { Task } from '../../../types';
import type { Timetable, TimetableTick } from './types';

/** Candidate axis intervals in seconds, from finest to coarsest. */
const TICK_STEPS = [
  1,
  2,
  5,
  10,
  15,
  30,
  MINUTE,
  2 * MINUTE,
  5 * MINUTE,
  10 * MINUTE,
  15 * MINUTE,
  30 * MINUTE,
  HOUR,
  2 * HOUR,
  3 * HOUR,
  6 * HOUR,
  12 * HOUR,
  DAY,
];

/** Upper bound on axis intervals, keeping tick labels legible at any width. */
const MAX_TICK_INTERVALS = 5;

/**
 * Turns a hierarchical task list into a flat gantt model: one row per task,
 * subtasks included, positioned on a timeline shared by every row.
 *
 * Tasks are placed relative to the earliest known start. A running task without
 * an end time is extended to `now`, and a task known by a single timestamp becomes
 * a zero-length point on the axis.
 *
 * @param taskList - Task tree to lay out
 * @param now - Current epoch seconds, injectable for deterministic rendering and tests
 */
export function buildTimetable<TTaskRecord extends object = object>(
  taskList: Task<TTaskRecord>[],
  now: number = Date.now() / 1000
): Timetable<TTaskRecord> {
  const entries = flattenTaskList(taskList).map((flatTask) => ({
    ...flatTask,
    ...resolveSpan(flatTask.task, now),
  }));

  const spans = entries.flatMap(({ start, end }) =>
    start === undefined || end === undefined ? [] : [{ start, end }]
  );
  if (!spans.length) {
    return {
      rows: entries.map(({ task, depth, ancestors }) => ({
        task,
        depth,
        ancestors,
        ongoing: false,
      })),
      totalSeconds: 0,
      ticks: [],
      hasTiming: false,
    };
  }

  const timelineStart = Math.min(...spans.map(({ start }) => start));
  const totalSeconds = Math.max(...spans.map(({ end }) => end)) - timelineStart;

  return {
    rows: entries.map(({ task, depth, ancestors, ongoing, start, end }) => ({
      task,
      depth,
      ancestors,
      ongoing,
      startOffset: start === undefined ? undefined : start - timelineStart,
      durationSeconds: start === undefined || end === undefined ? undefined : end - start,
    })),
    totalSeconds,
    ticks: buildTicks(totalSeconds),
    hasTiming: true,
  };
}

/**
 * Picks a readable axis interval for the given span and returns evenly spaced
 * ticks labelled as durations from the start of the timeline.
 */
export function buildTicks(totalSeconds: number): TimetableTick[] {
  if (totalSeconds <= 0) return [{ offsetSeconds: 0, label: formatDuration(0) }];

  const step =
    TICK_STEPS.find((candidate) => totalSeconds / candidate <= MAX_TICK_INTERVALS) ??
    Math.ceil(totalSeconds / MAX_TICK_INTERVALS / DAY) * DAY;

  const ticks: TimetableTick[] = [];
  for (let offsetSeconds = 0; offsetSeconds <= totalSeconds; offsetSeconds += step) {
    ticks.push({ offsetSeconds, label: formatDuration(offsetSeconds) });
  }

  return ticks;
}

function resolveSpan<TTaskRecord extends object>(
  task: Task<TTaskRecord>,
  now: number
): { ongoing: boolean; start?: number; end?: number } {
  const { start, end } = resolveTaskTiming(task);
  const resolvedStart = start ?? end;

  if (resolvedStart === undefined) {
    return { ongoing: false };
  }

  const ongoing = end === undefined && task.state === TASK_STATE.RUNNING;
  const resolvedEnd = Math.max(end ?? (ongoing ? now : resolvedStart), resolvedStart);

  return { ongoing, start: resolvedStart, end: resolvedEnd };
}
