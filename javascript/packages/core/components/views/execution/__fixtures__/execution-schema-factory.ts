import { merge } from 'lodash';

import { TASK_STATE } from '../constants';

import type { DeepPartial } from '#core/types/utility-types';
import type { ExecutionDetailViewSchema } from '../types';

/**
 * Factory for creating ExecutionDetailViewSchema test fixtures.
 * Provides minimal required properties for rendering with sensible defaults.
 *
 * @param base - Partial object shared across all test fixtures for a test suite
 * @returns Function that generates a complete execution schema using overrides.
 *
 * @example
 * ```typescript
 * // Setup base configuration for test suite
 * const buildSchema = buildExecutionSchemaFactory();
 * const basicSchema = buildSchema();
 *
 * // Custom variations
 * const customSchema = buildSchema({
 *   tasks: { accessor: 'pipeline.steps' },
 *   emptyState: { title: 'Custom Empty State' }
 * });
 * ```
 */
export const buildExecutionSchemaFactory = (base: DeepPartial<ExecutionDetailViewSchema> = {}) => {
  return (overrides: DeepPartial<ExecutionDetailViewSchema> = {}): ExecutionDetailViewSchema => {
    const required: ExecutionDetailViewSchema = {
      type: 'execution',
      emptyState: {
        title: 'No execution data',
        description: 'No tasks available',
      },
      tasks: {
        accessor: 'status.steps',
        subTasksAccessor: 'subSteps',
        header: {
          heading: 'displayName',
        },
        stateBuilder: (record: { state: string }) => {
          switch (record.state) {
            case 'PIPELINE_RUN_STEP_STATE_SUCCEEDED':
            case 'SUCCEEDED':
              return TASK_STATE.SUCCESS;
            case 'PIPELINE_RUN_STEP_STATE_RUNNING':
            case 'RUNNING':
              return TASK_STATE.RUNNING;
            case 'PIPELINE_RUN_STEP_STATE_FAILED':
            case 'FAILED':
              return TASK_STATE.ERROR;
            default:
              return TASK_STATE.PENDING;
          }
        },
      },
    };

    return merge({}, required, base, overrides);
  };
};

/**
 * Factory for creating task step data structures.
 *
 * @param base - Partial object shared across all test fixtures for a test suite
 * @returns Function that generates a complete task step using overrides.
 *
 * @example
 * ```typescript
 * const buildStep = buildTaskStepFactory();
 * const step = buildStep({ displayName: 'Build Task', state: 'RUNNING' });
 * ```
 */
export const buildTaskStepFactory = (base: Record<string, unknown> = {}) => {
  return (overrides: Record<string, unknown> = {}): Record<string, unknown> => {
    const required = {
      displayName: 'Default Task',
      state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
      subSteps: [],
    };

    return merge({}, required, base, overrides);
  };
};
