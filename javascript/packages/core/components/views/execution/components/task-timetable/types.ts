import type { Task } from '#core/components/views/execution/types';
import type { TimetableRow } from './utils/types';

export type TaskTimetableProps<TTaskRecord extends object = object> = {
  taskList: Task<TTaskRecord>[];
};

export type TaskTimetableBarProps<TTaskRecord extends object = object> = {
  row: TimetableRow<TTaskRecord>;
  totalSeconds: number;
};
