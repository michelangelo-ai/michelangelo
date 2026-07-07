import type { RowCell } from '#core/components/row/types';
import type { ExecutionOverrides, Task } from '#core/components/views/execution/types';
import type { TaskBodySchema } from './renderers/types';

export type TaskDetailsProps<
  TTaskRecord extends Record<string, unknown> = Record<string, unknown>,
> = {
  task: Task<TTaskRecord>;
  metadata?: RowCell[];
  bodySchema?: TaskBodySchema[];
  overrides?: ExecutionOverrides<TTaskRecord>;
};

export type TaskHeaderProps<TTaskRecord extends Record<string, unknown> = Record<string, unknown>> =
  {
    task: Task<TTaskRecord>;
    metadata?: RowCell[];
    id?: string;
  };

export type TaskBodyProps<TTaskRecord extends Record<string, unknown> = Record<string, unknown>> = {
  task: Task<TTaskRecord>;
  metadata?: RowCell[];
  bodySchema?: TaskBodySchema[];
  overrides?: ExecutionOverrides<TTaskRecord>;
};
