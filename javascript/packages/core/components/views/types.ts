import type { ReactNode } from 'react';
import type { Cell } from '#core/components/cell/types';
import type { EmptyState } from '#core/components/table/components/table-empty-state/types';
import type { PageSizeOption } from '#core/components/table/components/table-pagination/types';
import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { TableData } from '#core/components/table/types/data-types';
import type { TableProps as _TableProps } from '#core/components/table/types/table-types';
import type { DetailPageConfig } from '#core/components/views/detail-view/types/detail-view-schema-types';

export type MainViewContainerProps = {
  children: ReactNode;
};

export type ViewConfig<T extends object = object> = ListViewConfig<T> | DetailViewConfig<T>;

export interface ListViewConfig<T extends object = object> {
  type: 'list';
  tableConfig: TableConfig<T>;
}

export interface DetailViewConfig<T extends object = object> {
  type: 'detail';

  /** Metadata items to display in the detail view header */
  metadata: Cell[];

  /** Content sections to display in the detail view */
  pages: DetailPageConfig<T>[];
}

/**
 * Table configuration exposed to views. Can be used by detail view tables,
 * list views, form tables, etc.
 *
 * Defaults are inherited from {@link _TableProps}
 */
export interface TableConfig<T extends TableData = TableData> {
  columns: ColumnConfig<T>[];

  /** Content to display when the table has no data */
  emptyState?: EmptyState;

  disablePagination?: boolean;
  disableSorting?: boolean;
  disableSearch?: boolean;
  disableFilters?: boolean;

  /** Available page sizes for the table */
  pageSizes?: PageSizeOption[];

  /** Whether to enable sticky sides in the table */
  enableStickySides?: boolean;

  /** Optional actions component for the table */
  actions?: React.ComponentType<{ record: T }>;
}
