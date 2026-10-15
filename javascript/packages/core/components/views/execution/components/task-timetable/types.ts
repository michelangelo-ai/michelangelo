import type { Task } from '#core/components/views/execution/types';
import type { TimetableRow } from './utils/types';

export type TaskTimetableProps<TTaskRecord extends object = object> = {
  taskList: Task<TTaskRecord>[];
  /** Receives the clicked task plus its ancestors, ordered from root to direct parent */
  onTaskClick: (task: Task<TTaskRecord>, ancestors: Task<TTaskRecord>[]) => void;
};

export type TaskTimetableBarProps<TTaskRecord extends object = object> = {
  row: TimetableRow<TTaskRecord>;
  totalSeconds: number;
};
