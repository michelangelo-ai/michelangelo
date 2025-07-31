import type { Accessor } from '#core/types/common/studio-types';
import type { TASK_STATE } from './constants';

/**
 * Configuration schema for rendering execution views that display hierarchical task lists.
 * Used to transform raw execution data into structured task representations with state tracking.
 *
 * @template TData - The shape of the input data containing task records
 * @template TTaskRecord - The shape of individual raw task records before processing
 *
 * @example
 * ```typescript
 * // Basic pipeline execution schema
 * const pipelineSchema: ExecutionDetailViewSchema<PipelineData, PipelineStep> = {
 *   type: 'execution',
 *   emptyState: {
 *     title: 'No pipeline steps',
 *     description: 'This pipeline has no execution steps to display'
 *   },
 *   tasks: {
 *     accessor: 'status.steps',
 *     subTasksAccessor: 'subSteps',
 *     header: { heading: 'displayName' },
 *     stateBuilder: (step) => step.state === 'SUCCEEDED' ? TASK_STATE.SUCCESS : TASK_STATE.ERROR
 *   }
 * };
 * ```
 */
export type ExecutionDetailViewSchema<
  TData extends object = object,
  TTaskRecord extends object = object,
> = {
  type: 'execution';

  /**
   * Content displayed when no tasks are found.
   * Shows when the accessor returns an empty array or no data.
   */
  emptyState: {
    /** Primary message shown when no tasks exist */
    title: string;
    /** Optional additional context about why no tasks are available */
    description?: string;
  };

  /**
   * Configuration for extracting and processing task data from the input.
   * Defines how to locate tasks, extract names, and determine states.
   */
  tasks: {
    /**
     * Extracts the array of raw task records from the input data.
     * Can be a string path (e.g., 'status.steps') or function.
     *
     * @example
     * ```typescript
     * // String accessor for nested data
     * accessor: 'pipeline.execution.steps'
     *
     * // Function accessor for complex logic
     * accessor: (data) => data.workflow?.tasks || []
     * ```
     */
    accessor: Accessor<TTaskRecord[]>;

    /**
     * Optional accessor to extract child tasks from each task record.
     * Enables hierarchical task structures with parent/child relationships.
     * If not provided, tasks are treated as flat list.
     *
     * @example
     * ```typescript
     * // Simple property access
     * subTasksAccessor: 'subSteps'
     *
     * // Complex nested extraction
     * subTasksAccessor: (task) => task.children?.filter(child => child.visible)
     * ```
     */
    subTasksAccessor?: Accessor<TTaskRecord[]>;

    /**
     * Configuration for extracting display information from task records.
     */
    header: {
      /**
       * Extracts the display name for each task.
       * Falls back to 'name' property if accessor returns falsy value.
       *
       * @example
       * ```typescript
       * // Simple property access
       * heading: 'displayName'
       *
       * // Computed display name
       * heading: (task) => `${task.type}: ${task.name}`
       * ```
       */
      heading: Accessor<string>;
    };

    /**
     * Transforms raw task records into standardized task states.
     * Called for each task to determine its execution status.
     *
     * @param taskRecord - The raw task record being processed
     * @param taskIndex - Position of this task in the sibling array
     * @param siblingTasks - Array of all sibling task records
     * @param rootData - The original input data for context
     * @returns Standardized task state from TASK_STATE constants
     *
     * @example
     * ```typescript
     * stateBuilder: (step, index, siblings, pipelineData) => {
     *   if (step.status === 'COMPLETED') return TASK_STATE.SUCCESS;
     *   if (step.status === 'FAILED') return TASK_STATE.ERROR;
     *   if (step.status === 'RUNNING') return TASK_STATE.RUNNING;
     *   return TASK_STATE.PENDING;
     * }
     * ```
     */
    stateBuilder: (
      taskRecord: TTaskRecord,
      taskIndex: number,
      siblingTasks: TTaskRecord[],
      rootData: TData
    ) => TaskState;
  };
};

export type TaskState = (typeof TASK_STATE)[keyof typeof TASK_STATE];

/**
 * Processed task representation with standardized properties and hierarchy.
 * Output of buildTaskList after transforming raw task records.
 *
 * @template TTaskRecord - The shape of the original raw task record
 */
export type Task<TTaskRecord extends object = object> = {
  /** Display name extracted from the raw task record */
  name: string;
  /** Standardized execution state from TASK_STATE constants */
  state: TaskState;
  /** Child tasks in hierarchical structures */
  subTasks: Task<TTaskRecord>[];
  /** Original raw task record for accessing additional properties */
  record: TTaskRecord;
  /** True for the first non-completed task */
  active: boolean;
};
