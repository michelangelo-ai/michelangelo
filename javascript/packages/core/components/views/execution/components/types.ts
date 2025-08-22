import type { Task } from '#core/components/views/execution/types';

export type TaskFlowProps<TTaskRecord extends object = object> = {
  matrix: TaskMatrixItem<TTaskRecord>[];
  onTaskClick: (task: Task<TTaskRecord>) => void;
};

export type TaskMatrixItem<TTaskRecord extends object = object> = {
  parent?: Task<TTaskRecord>;
  taskList: Task<TTaskRecord>[];
};
