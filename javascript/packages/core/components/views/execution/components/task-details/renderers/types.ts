import type { Accessor } from '#core/types/common/studio-types';

export type TaskBodySchema = SharedTaskBodySchema;

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
