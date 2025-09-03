import type { ExecutionDetailViewSchema } from '#core/components/views/execution/types';

export type DetailPageConfig<T extends object = object> =
  | BaseDetailPageConfig
  | ExecutionDetailPageConfig<T>
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

export interface CustomDetailPageConfig<T extends object = object> extends BaseDetailPageConfig {
  component: React.ComponentType<{ data: T | undefined; isLoading: boolean }>;
  type: 'custom';
}
