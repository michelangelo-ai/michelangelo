import { get } from 'lodash';

import { getObjectValue } from '#core/utils/object-utils';
import { TASK_STATE } from '../constants';

import type { ExecutionDetailViewSchema, Task } from '../types';

/**
 * Transforms raw execution data into structured task representations with hierarchy and state tracking.
 * Extracts tasks using schema accessors, builds parent/child relationships, and determines focused states.
 *
 * @note
 * `focused` state is determined by the first non-completed task in the list. If all tasks are completed,
 * the last task is considered focused.
 *
 * @param schema - Configuration defining how to extract and process task data
 * @param data - Raw input data containing task records
 *
 * @example
 * ```typescript
 * const tasks = buildTaskList(pipelineSchema, pipelineData);
 * // Returns: [{ name: 'Build', state: 'SUCCESS', focused: false, subTasks: [...] }]
 * ```
 */
export function buildTaskList<TData extends object, TTaskRecord extends object>(
  schema: ExecutionDetailViewSchema<TData, TTaskRecord>,
  data: TData
): Task<TTaskRecord>[] {
  const { tasks } = schema;
  const { accessor, subTasksAccessor } = tasks;

  function buildTask(
    taskRecord: TTaskRecord,
    taskIndex: number,
    siblingTasks: TTaskRecord[]
  ): Task<TTaskRecord> {
    const focusedItemIndex = siblingTasks.findIndex((item, idx) => {
      const state = schema.tasks.stateBuilder(item, idx, siblingTasks, data);
      return state !== TASK_STATE.SUCCESS && state !== TASK_STATE.SKIPPED;
    });

    return {
      name: (getObjectValue(taskRecord, tasks.header.heading) ?? get(taskRecord, 'name'))!,
      state: schema.tasks.stateBuilder(taskRecord, taskIndex, siblingTasks, data),
      subTasks: subTasksAccessor
        ? getObjectValue(taskRecord, subTasksAccessor, [])!.map(buildTask)
        : [],
      record: taskRecord,
      focused:
        focusedItemIndex === taskIndex ||
        (focusedItemIndex === -1 && taskIndex === siblingTasks.length - 1),
    };
  }

  const taskArray = getObjectValue(data, accessor, [])!;
  return taskArray.map(buildTask);
}
