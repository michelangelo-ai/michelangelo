import type { ExecutionOverrides, Task } from '#core/components/views/execution/types';

export type TaskFlowProps<TTaskRecord extends Record<string, unknown> = Record<string, unknown>> = {
  matrix: TaskMatrixItem<TTaskRecord>[];
  onTaskClick: (task: Task<TTaskRecord>) => void;
  overrides?: Pick<ExecutionOverrides<TTaskRecord>, 'TaskListRenderer' | 'SubTaskListRenderer'>;
};

export type TaskListRendererProps<
  TTaskRecord extends Record<string, unknown> = Record<string, unknown>,
> = {
  taskList: Task<TTaskRecord>[];
  onTaskClick: (task: Task<TTaskRecord>) => void;
  parent?: Task<TTaskRecord>;
};

export type TaskMatrixItem<TTaskRecord extends Record<string, unknown> = Record<string, unknown>> =
  {
    parent?: Task<TTaskRecord>;
    taskList: Task<TTaskRecord>[];
  };
