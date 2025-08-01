import { merge } from 'lodash';

import { TASK_STATE } from '#core/components/views/execution/constants';

import type { ExecutionDetailViewSchema } from '#core/components/views/execution/types';
import type { DeepPartial } from '#core/types/utility-types';

/**
 * Creates a raw task record object for testing buildTaskList functionality.
 * Returns the input data format that buildTaskList expects, not the processed Task type.
 *
 * @param name - The display name of the task
 * @param state - The execution state (e.g., 'SUCCEEDED', 'FAILED', 'RUNNING')
 * @param subSteps - Array of child task records for hierarchical testing
 * @param overrides - Additional properties to merge into the task record
 * @returns Raw task record object (not the processed Task type)
 *
 * @example
 * ```typescript
 * const taskRecord = createTask('Build Image', 'SUCCEEDED', [
 *   createTask('Compile', 'SUCCEEDED'),
 *   createTask('Package', 'RUNNING')
 * ]);
 * // Use with buildTaskList to get actual Task objects
 * const tasks = buildTaskList(schema, createStepsData([taskRecord]));
 * ```
 */
export const createTask = (
  name: string,
  state: string,
  subSteps: object[] = [],
  overrides: Record<string, unknown> = {}
) => ({
  displayName: name,
  state,
  subSteps,
  ...overrides,
});

/**
 * Creates an ExecutionDetailViewSchema for testing buildTaskList behavior.
 * Provides sensible defaults with the ability to override specific configuration.
 *
 * @param overrides - Partial schema properties to customize behavior
 *
 * @example
 * ```typescript
 * // Basic schema with default stateBuilder
 * const schema = createSchema();
 *
 * // Custom accessor pattern
 * const customSchema = createSchema({
 *   tasks: {
 *     accessor: 'pipeline.steps',
 *     header: { heading: (record) => record.name }
 *   }
 * });
 * ```
 */
export const createSchema = (
  overrides: DeepPartial<ExecutionDetailViewSchema> = {}
): ExecutionDetailViewSchema =>
  merge(
    {
      type: 'execution',
      emptyState: {
        title: 'No tasks',
        description: 'No tasks available',
      },
      tasks: {
        accessor: 'steps',
        subTasksAccessor: 'subSteps',
        header: {
          heading: 'displayName',
        },
        stateBuilder: (record: { state: string }) => {
          switch (record.state) {
            case 'SUCCEEDED':
              return TASK_STATE.SUCCESS;
            case 'FAILED':
              return TASK_STATE.ERROR;
            case 'RUNNING':
              return TASK_STATE.RUNNING;
            default:
              return TASK_STATE.PENDING;
          }
        },
      },
    },
    overrides
  );
