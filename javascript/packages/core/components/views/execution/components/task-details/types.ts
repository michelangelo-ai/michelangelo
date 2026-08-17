import type { ActionConfigSchema, Data } from '#core/components/actions/types';
import type { RowCell } from '#core/components/row/types';
import type { ExecutionOverrides, Task } from '#core/components/views/execution/types';
import type { TaskBodySchema } from './renderers/types';

export type TaskDetailsProps<TTaskRecord extends object = object> = {
  task: Task<TTaskRecord>;
  metadata?: RowCell[];
  actions?: ActionConfigSchema<Data>[];
  pageData?: Data;
  bodySchema?: TaskBodySchema[];
  overrides?: ExecutionOverrides<TTaskRecord>;
};

export type TaskHeaderProps<TTaskRecord extends object = object> = {
  task: Task<TTaskRecord>;
  metadata?: RowCell[];
  actions?: ActionConfigSchema<Data>[];
  pageData?: Data;
  id?: string;
};

export type TaskBodyProps<TTaskRecord extends object = object> = {
  task: Task<TTaskRecord>;
  metadata?: RowCell[];
  actions?: ActionConfigSchema<Data>[];
  pageData?: Data;
  bodySchema?: TaskBodySchema[];
  overrides?: ExecutionOverrides<TTaskRecord>;
};
