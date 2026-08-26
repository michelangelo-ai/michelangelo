import type { TableConfig } from '#core/components/views/types';
import type { QueryConfig } from '#core/types/query-types';

export interface DetailViewTablePageProps<T extends object = object> {
  /** Query configuration for fetching data to display in the table */
  queryConfig: QueryConfig;

  tableConfig: TableConfig<T>;

  /** Unique page identifier for table state persistence */
  pageId: string;

  /** Whether the parent detail view is loading, preventing table queries */
  isDetailViewLoading?: boolean;
}
