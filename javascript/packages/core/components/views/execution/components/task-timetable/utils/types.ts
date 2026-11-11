import type { Task } from '../../../types';

export type FlatTask<TTaskRecord extends object = object> = {
  task: Task<TTaskRecord>;
  /** Nesting level, 0 for top-level tasks */
  depth: number;
  /** Ancestors ordered from root to direct parent */
  ancestors: Task<TTaskRecord>[];
};

export type TaskTiming = {
  /** Epoch seconds at which the task started */
  start?: number;
  /** Epoch seconds at which the task finished */
  end?: number;
};

export type TimetableRow<TTaskRecord extends object = object> = FlatTask<TTaskRecord> & {
  /** Seconds between the start of the timeline and the start of this task */
  startOffset?: number;
  /** Length of the task in seconds, 0 for tasks known only by a single timestamp */
  durationSeconds?: number;
  /** True when the task is still running and its bar is extended to the current time */
  ongoing: boolean;
};

export type TimetableTick = {
  /** Seconds from the start of the timeline */
  offsetSeconds: number;
  label: string;
};

export type Timetable<TTaskRecord extends object = object> = {
  rows: TimetableRow<TTaskRecord>[];
  /** Span covered by the timeline, in seconds */
  totalSeconds: number;
  ticks: TimetableTick[];
  /** False when no task in the tree carries any usable timestamp */
  hasTiming: boolean;
};
