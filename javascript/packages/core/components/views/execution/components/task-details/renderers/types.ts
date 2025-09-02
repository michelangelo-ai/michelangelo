import type { RowCell } from '#core/components/row/types';
import type { Accessor } from '#core/types/common/studio-types';

export type TaskBodySchema = TaskBodyStructSchema | TaskBodyTextareaSchema | TaskBodyMetadataSchema;

export interface SharedTaskBodySchema {
  /**
   * Controls how the body content is rendered
   *
   * @example 'struct'
   */
  type: string;

  label: string;
}

export interface TaskBodyStructSchema extends SharedTaskBodySchema {
  type: 'struct';

  /**
   * Used to access the value of the body content
   *
   * @example 'spec.content.metadata.name'
   * @example (task) => task.input
   */
  accessor: Accessor<object>;
}

export interface TaskBodyTextareaSchema extends SharedTaskBodySchema {
  type: 'textarea';

  error?: boolean;
  markdown?: boolean;

  /**
   * Used to access the value of the body content
   *
   * @example 'spec.content.metadata.name'
   * @example (task) => task.input
   */
  accessor: Accessor<string>;
}

export interface TaskBodyMetadataSchema extends SharedTaskBodySchema {
  type: 'metadata';

  cells: RowCell[];
}

export interface TaskBodyStructProps extends Omit<TaskBodyStructSchema, 'type' | 'accessor'> {
  value?: object;
}

export interface TaskBodyTextAreaProps extends Omit<TaskBodyTextareaSchema, 'type' | 'accessor'> {
  value?: string;
}

export interface TaskBodyMetadataProps extends Omit<TaskBodyMetadataSchema, 'type'> {
  value?: Record<string, unknown>;
}
