import type { Cell } from '#core/components/cell/types';
import type { ExecutionOverrides, Task } from '#core/components/views/execution/types';
import type { TaskBodySchema } from './renderers/types';

export type TaskDetailsProps<TTaskRecord extends object = object> = {
  task: Task<TTaskRecord>;
  metadata?: Cell[];
  bodySchema?: TaskBodySchema[];
  overrides?: ExecutionOverrides<TTaskRecord>;
};

export type TaskHeaderProps<TTaskRecord extends object = object> = {
  task: Task<TTaskRecord>;
  metadata?: Cell[];
  id?: string;
};

export type TaskBodyProps<TTaskRecord extends object = object> = {
  task: Task<TTaskRecord>;
  bodySchema?: TaskBodySchema[];
  overrides?: ExecutionOverrides<TTaskRecord>;
};
