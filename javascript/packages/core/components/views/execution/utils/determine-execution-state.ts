import { TASK_STATE } from '../constants';

import type { Task, TaskState } from '../types';

export function determineExecutionState(taskList: Task[]): TaskState {
  if (!taskList.length) {
    return TASK_STATE.PENDING;
  }
  if (taskList.some(({ state }) => state === TASK_STATE.RUNNING)) {
    return TASK_STATE.RUNNING;
  }
  if (taskList.some(({ state }) => state === TASK_STATE.ERROR)) {
    return TASK_STATE.ERROR;
  }

  return taskList.at(-1)?.state ?? TASK_STATE.PENDING;
}
