import type { Task } from '../../../types';
import type { FlatTask } from './types';

/**
 * Flattens a hierarchical task list into a depth-first ordered list, keeping the
 * nesting level and ancestor chain of every task.
 *
 * @example
 * ```typescript
 * flattenTaskList([{ name: 'Build', subTasks: [{ name: 'Compile', subTasks: [] }] }]);
 * // [{ task: Build, depth: 0, ancestors: [] }, { task: Compile, depth: 1, ancestors: [Build] }]
 * ```
 */
export function flattenTaskList<TTaskRecord extends object = object>(
  taskList: Task<TTaskRecord>[],
  ancestors: Task<TTaskRecord>[] = []
): FlatTask<TTaskRecord>[] {
  return taskList.flatMap((task) => [
    { task, depth: ancestors.length, ancestors },
    ...flattenTaskList(task.subTasks, [...ancestors, task]),
  ]);
}
