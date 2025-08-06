import { TaskFlow } from '../task-flow';

import type { TaskBodyProps } from './types';

export function TaskBody<TTaskRecord extends object>(props: TaskBodyProps<TTaskRecord>) {
  const { task } = props;
  const { subTasks } = task;

  if (!subTasks?.length) {
    return null;
  }

  return <TaskFlow taskList={subTasks} />;
}
