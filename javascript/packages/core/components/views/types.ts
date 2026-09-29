import type { ReactNode } from 'react';
import type { ActionConfigSchema } from '#core/components/actions/types';
import type { RowCell } from '#core/components/row/types';
import type { EmptyState } from '#core/components/table/components/table-empty-state/types';
import type { PageSizeOption } from '#core/components/table/components/table-pagination/types';
import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { TableData } from '#core/components/table/types/data-types';
import type { TableProps as _TableProps } from '#core/components/table/types/table-types';
import type { DetailPageConfig } from '#core/components/views/detail-view/types/detail-view-schema-types';
import type { QueryConfig } from '#core/types/query-types';

export type MainViewContainerProps = {
  children: ReactNode;
};

export type ViewConfig<T extends object = object> = ListViewConfig<T> | DetailViewConfig<T>;

export interface ListViewConfig<T extends object = object> {
  type: 'list';
  tableConfig: TableConfig<T>;

  /**
   * Alternate data sources the list can be toggled between, rendered as a segmented
   * control in the table action bar. The first variant is the default; a variant that
   * omits `service`/`tableConfig` falls back to the entity's own.
   *
   * @example
   * ```ts
   * variants: [
   *   { id: 'pipelines', label: 'Pipelines' },
   *   { id: 'revisions', label: 'Revisions', service: 'revision', tableConfig: REVISION_TABLE },
   * ]
   * ```
   */
  variants?: ListViewVariant<T>[];
}

/** One selectable data source of a {@link ListViewConfig} with `variants`. */
export interface ListViewVariant<T extends object = object> {
  /** Stable id, used to namespace persisted table state */
  id: string;
  /** Segment label */
  label: string;
  /** Service to list from; defaults to the entity's service */
  service?: QueryConfig['service'];
  /** Table configuration for this variant; defaults to the list view's `tableConfig` */
  tableConfig?: TableConfig<T>;
}

export interface DetailViewConfig<T extends object = object> {
  type: 'detail';

  /**
   * Metadata items to display in the detail view header, rendered by {@link Row} — accepts
   * {@link RowCell}'s `hideEmpty` (e.g. mutually-exclusive fields that should disappear
   * entirely rather than render blank) in addition to the base {@link Cell} shape.
   */
  metadata: RowCell[];

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

  /** Caps the scrollable table area's height (CSS length); rows beyond it scroll under a pinned header */
  maxHeight?: string;

  /** Optional actions to render in each table row */
  actions?: ActionConfigSchema<T>[];
}
