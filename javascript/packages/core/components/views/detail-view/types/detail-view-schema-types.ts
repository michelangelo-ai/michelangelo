import type { ExecutionDetailViewSchema } from '#core/components/views/execution/types';
import type { TableConfig } from '#core/components/views/types';
import type { QueryConfig } from '#core/types/query-types';

export type DetailPageConfig<T extends object = object> =
  | BaseDetailPageConfig
  | ExecutionDetailPageConfig<T>
  | TableDetailPageConfig<T>
  | CustomDetailPageConfig<T>;

interface BaseDetailPageConfig {
  /** Type of page content to render */
  type: string;

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

  tableConfig: TableConfig<T>;
}

export interface CustomDetailPageConfig<T extends object = object> extends BaseDetailPageConfig {
  component: React.ComponentType<{ data: T | undefined; isLoading: boolean }>;
  type: 'custom';
}
