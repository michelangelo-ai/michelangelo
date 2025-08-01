import type { Task } from '../types';

/**
 * Builds a matrix representation of tasks for inline rendering.
 *
 * In order to render the taskList in a vertical list,
 * where each line is a list of tasks that are executed sequentially,
 * and sub lines are children tasks of one item in its parent line (recursively)
 *
 * @param taskList - Array of tasks to process
 * @param parent - Optional parent task for context
 * @returns Array of matrix rows with parent and taskList
 */
export function buildMatrix<TTaskRecord extends object = object>(
  taskList: Task<TTaskRecord>[],
  parent?: Task<TTaskRecord>
): { parent?: Task<TTaskRecord>; taskList: Task<TTaskRecord>[] }[] {
  if (!taskList.length) return [];

  const focusedTask =
    taskList.find((task: Task<TTaskRecord>) => task.focused) ?? taskList[taskList.length - 1];

  if (focusedTask.subTasks.length === 0) return [{ parent, taskList }];

  return [{ parent, taskList }, ...buildMatrix(focusedTask.subTasks, focusedTask)];
}
