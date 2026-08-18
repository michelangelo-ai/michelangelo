import type { ComponentType, ReactNode } from 'react';
import type { ActionConfigSchema, SuccessOperation } from '#core/components/actions/types';
import type { Cell } from '#core/components/cell/types';
import type { FormConfig } from '#core/components/form/types/config-types';
import type { EmptyState } from '#core/components/table/components/table-empty-state/types';
import type { PageSizeOption } from '#core/components/table/components/table-pagination/types';
import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { TableData } from '#core/components/table/types/data-types';
import type { TableProps as _TableProps } from '#core/components/table/types/table-types';
import type { DetailPageConfig } from '#core/components/views/detail-view/types/detail-view-schema-types';
import type { MiddlewareSchema } from '#core/hooks/use-schema-middleware/types';

export type MainViewContainerProps = {
  children: ReactNode;
};

export type ViewConfig<T extends object = object> =
  | ListViewConfig<T>
  | DetailViewConfig<T>
  | FormViewConfig;

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

  /** Optional actions to render in each table row */
  actions?: ActionConfigSchema<T>[];
}

export type FormOperation = 'create' | 'update';

export type FormViewConfigSchema = {
  type: 'form';
  config: FormConfig;
  subType?: { path: string; value: string };
  successOperations?: SuccessOperation[];
  middleware?: MiddlewareSchema;
};

export type FormViewConfigComponent = {
  type: 'form';
  component: ComponentType<{ record?: unknown; formOperation: FormOperation }>;
  subType?: { path: string; value: string };
};

export type FormViewConfig = FormViewConfigSchema | FormViewConfigComponent;
