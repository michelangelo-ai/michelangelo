import type { ExecutionDetailViewSchema } from '#core/components/views/execution/types';
import type { TableConfig } from '#core/components/views/types';
import type { QueryConfig } from '#core/types/query-types';

export type DetailPageConfig<T extends object = object> =
  | ExecutionDetailPageConfig<T>
  | TableDetailPageConfig<T>
  | CustomDetailPageConfig<T>;

interface BaseDetailPageConfig {
  /** Unique identifier for the page, used for entityTab param in the URL */
  id: string;

  /** Label to be displayed in the detail view header */
  label: string;
}

export interface ExecutionDetailPageConfig<T extends object = object>
  extends BaseDetailPageConfig,
    ExecutionDetailViewSchema<T> {
  type: 'execution';
}

export interface TableDetailPageConfig<T extends object = object> extends BaseDetailPageConfig {
  type: 'table';

  /** Query configuration for fetching data to display in the table */
  queryConfig: QueryConfig;

  /**
   * Narrows the fetched rows client-side. Use for relationships the API exposes as a spec
   * field rather than a label, which `queryConfig.serviceOptions.listOptions.labelSelector`
   * cannot express.
   */
  filter?: TableDetailPageFilter;

  tableConfig: TableConfig<T>;
}

export interface TableDetailPageFilter {
  /** Dot path read from each row, e.g. `spec.pipeline.name` */
  field: string;

  /** Row is kept when the value at `field` matches this */
  equals: string;
}

export interface CustomDetailPageConfig<T extends object = object> extends BaseDetailPageConfig {
  component: React.ComponentType<{ data: T | undefined; isLoading: boolean }>;
  type: 'custom';
}
