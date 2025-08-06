import type { Task } from '#core/components/views/execution/types';

export type TaskStepCardProps<TTaskRecord extends object = object> = {
  task: Task<TTaskRecord>;
  onClick?: () => void;
};
