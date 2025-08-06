import { merge } from 'lodash';

import { TASK_STATE } from '../../../constants';

import type { DeepPartial } from '#core/types/utility-types';
import type { Task } from '../../../types';

/**
 * Creates a Task object for testing TaskDetails component functionality.
 * Merges provided overrides with sensible defaults for easy test data creation.
 *
 * @param overrides - Partial Task properties to customize the created task
 * @returns Task object for use in component tests
 *
 * @example
 * ```typescript
 * const leafTask = createTask({ name: 'Build Step' });
 * const parentTask = createTask({
 *   name: 'Pipeline',
 *   state: TASK_STATE.RUNNING,
 *   subTasks: [
 *     createTask({ name: 'Compile', state: TASK_STATE.SUCCESS }),
 *     createTask({ name: 'Test', state: TASK_STATE.RUNNING })
 *   ]
 * });
 *
 * // Task with metadata
 * const taskWithMetadata = createTask({
 *   name: 'Deploy',
 *   record: {
 *     duration: '300s',
 *     startTime: '2025-01-01T10:00:00Z'
 *   }
 * });
 * ```
 */
export const createTask = (overrides: DeepPartial<Task> = {}): Task =>
  merge(
    {
      name: 'Default Task',
      state: TASK_STATE.SUCCESS,
      subTasks: [],
      record: { displayName: 'Default Task', state: 'SUCCEEDED' },
      focused: false,
    },
    overrides
  );
