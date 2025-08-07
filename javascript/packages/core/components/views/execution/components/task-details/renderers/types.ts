import type { RowCell } from '#core/components/row/types';
import type { Accessor } from '#core/types/common/studio-types';

export type TaskBodySchema = SharedTaskBodySchema | TaskBodyTextareaSchema | TaskBodyMetadataSchema;

export interface SharedTaskBodySchema {
  /**
   * Controls how the body content is rendered
   *
   * @example 'struct'
   */
  type: string;

  label: string;

  /**
   * Used to access the value of the body content
   *
   * @example 'spec.content.metadata.name'
   * @example (task) => task.input
   */
  accessor: Accessor<unknown>;
}

export interface TaskBodyTextareaSchema extends SharedTaskBodySchema {
  error?: boolean;
  markdown?: boolean;
}

export interface TaskBodyMetadataSchema extends SharedTaskBodySchema {
  cells: RowCell[];
}

export interface TaskBodyTextAreaProps extends Omit<TaskBodyTextareaSchema, 'type' | 'accessor'> {
  value?: string;
}

export interface TaskBodyMetadataProps extends Omit<TaskBodyMetadataSchema, 'type' | 'accessor'> {
  value?: Record<string, unknown>;
}
