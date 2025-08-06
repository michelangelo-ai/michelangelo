import type { Cell } from '#core/components/cell/types';
import type { Task } from '#core/components/views/execution/types';
import type { TaskBodySchema } from './renderers/types';

export type TaskDetailsProps<TTaskRecord extends object = object> = {
  task: Task<TTaskRecord>;
  onClick?: () => void;
  metadata?: Cell[];
  bodySchema?: TaskBodySchema[];
};

export type TaskHeaderProps<TTaskRecord extends object = object> = {
  task: Task<TTaskRecord>;
  onClick?: () => void;
  metadata?: Cell[];
};

export type TaskBodyProps<TTaskRecord extends object = object> = {
  task: Task<TTaskRecord>;
  bodySchema?: TaskBodySchema[];
};
