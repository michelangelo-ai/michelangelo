import type { Cell } from '#core/components/cell/types';
import type { Task } from '../../types';

export type TaskDetailsProps<TTaskRecord extends object = object> = {
  task: Task<TTaskRecord>;
  onClick?: () => void;
  metadata?: Cell[];
};

export type TaskHeaderProps<TTaskRecord extends object = object> = {
  task: Task<TTaskRecord>;
  onClick?: () => void;
  metadata?: Cell[];
};

export type TaskBodyProps<TTaskRecord extends object = object> = {
  task: Task<TTaskRecord>;
};
