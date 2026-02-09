import type { ComponentType } from 'react';
import type { RowCell } from '#core/components/row/types';
import type { Accessor } from '#core/types/common/studio-types';
import type { TaskBodySchema } from './components/task-details/renderers/types';
import type { TaskListRendererProps } from './components/types';
import type { TASK_STATE } from './constants';

export type TaskState = (typeof TASK_STATE)[keyof typeof TASK_STATE];

export type StateStyleConfig = {
  borderColorName: string;
  backgroundColorName?: string;
  colorName?: string;
};

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

      /**
       * Optional metadata fields to display as rich content below the task heading.
       * Each field uses cell renderers for consistent formatting (dates, states, etc.).
       *
       * @example
       * ```typescript
       * metadata: [
       *   { id: 'lastUpdatedTimestamp', label: 'Last updated', type: 'DATE', accessor: 'lastUpdated' },
       *   { id: 'status', label: 'State', type: 'STATE', accessor: 'status' }
       * ]
       * ```
       */
      metadata?: RowCell[];
    };

    /**
     * Optional array of body content configurations for rendering detailed task information.
     * Each configuration specifies how to extract and display specific aspects of task data.
     *
     * @example
     * ```typescript
     * body: [
     *   { type: 'struct', label: 'Input', accessor: 'input' },
     *   { type: 'struct', label: 'Output', accessor: 'output' }
     * ]
     * ```
     */
    body?: TaskBodySchema[];

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
  /** True for the task that should receive UI focus and attention */
  focused: boolean;
};

/**
 * BaseUI-style overrides for Execution component
 *
 * Enables customization of execution rendering for different pipeline types:
 * - taskList: Override task data (bypasses buildTaskList)
 * - TaskListRenderer: Override how each taskList within matrix items is rendered
 *
 * Example usage for ASL pipeline types:
 * ```
 * const overrides = {
 *   taskList: enhancedTasksWithASL,
 *   TaskListRenderer: { component: ASLTaskListRenderer }
 * };
 * ```
 */
export type ExecutionOverrides<TTaskRecord extends object = object> = {
  TaskListRenderer?: {
    component?: ComponentType<TaskListRendererProps<TTaskRecord>>;
    props?: Partial<TaskListRendererProps<TTaskRecord>>;
  };
  taskList?: Task<TTaskRecord>[];
};
