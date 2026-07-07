import type { Task } from '#core/components/views/execution/types';

export type TaskStepCardProps<
  TTaskRecord extends Record<string, unknown> = Record<string, unknown>,
> = {
  task: Task<TTaskRecord>;
  onClick?: () => void;
};
