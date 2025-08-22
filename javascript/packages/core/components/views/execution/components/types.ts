import type { ExecutionOverrides, Task } from '#core/components/views/execution/types';

export type TaskFlowProps<TTaskRecord extends object = object> = {
  matrix: TaskMatrixItem<TTaskRecord>[];
  onTaskClick: (task: Task<TTaskRecord>) => void;
  overrides?: Pick<ExecutionOverrides<TTaskRecord>, 'TaskListRenderer'>;
};

export type TaskListRendererProps<TTaskRecord extends object = object> = {
  taskList: Task<TTaskRecord>[];
  onTaskClick: (task: Task<TTaskRecord>) => void;
  parent?: Task<TTaskRecord>;
};

export type TaskMatrixItem<TTaskRecord extends object = object> = {
  parent?: Task<TTaskRecord>;
  taskList: Task<TTaskRecord>[];
};
