import { EmptyState } from '../components/table-empty-state/types';

import type { ApplicationError } from '#core/types/error-types';
import type { TableActionBarConfig } from '../components/table-action-bar/types';
import type { TableBodyProps, TableRow } from '../components/table-body/types';
import type { PageSizeOption, TablePaginationProps } from '../components/table-pagination/types';
import type { ColumnConfig } from './column-types';
import type { TableData } from './data-types';

/**
 * @interface TableRequiredUserProps
 *
 * @description
 * Minimal props a user of `Table` must provide to render a table
 */
export interface TableRequiredUserProps<T extends TableData = TableData> {
  /**
   * @description
   * The data to be displayed in the table.
   */
  data: Array<T>;

  /**
   * @description
   * Columns to display in the table
   *
   * @example [{ id: 'name', label: 'Name' }, { id: 'age', label: 'Age' }]
   */
  columns: ColumnConfig<T>[];
}

/**
 * @interface TableRequiredFunctionalityProps
 *
 * @description
 * Required props for `Table` and its sub-components to render. Users
 * may omit these props, in which case a default value will be provided.
 */
export interface TableRequiredFunctionalityProps {
  /**
   * @description Configure landing card when the table has no data
   *
   * @default
   * ```ts
   * {
   *   title: 'No data',
   *   content: 'No data is present.',
   * }
   * ```
   */
  emptyState: EmptyState;

  /**
   * @description
   * If true, the table will hide the columns and display a loading state.
   * @default false
   */
  loading: boolean;

  /**
   * @description
   * View to display when the table is loading
   * @default TableLoadingState
   */
  loadingView: React.ComponentType;

  /**
   * @description
   * The query error associated with the data fetched for table population. Controls
   * whether an error state should be displayed within the table.
   *
   * @default undefined
   */
  error: ApplicationError | undefined;

  /**
   * @description
   * Configuration for the action bar above the table.
   * Controls search functionality and other action bar features.
   *
   * @default { enableSearch: true }
   */
  actionBarConfig: TableActionBarConfig;

  /**
   * @description
   * If true, table will display all data in a single page without pagination controls.
   *
   * @default false
   */
  disablePagination: boolean;

  /**
   * @description
   * If true, all columns will be non-sortable and sorting functionality will be disabled.
   *
   * @default false
   */
  disableSorting: boolean;

  /**
   * @description
   * Available page sizes for the table, formatted to provide to a dropdown
   * so user can modify page size during runtime.
   *
   * @default [{ id: 15, label: '15' }, { id: 25, label: '25' }, { id: 50, label: '50' }]
   */
  pageSizes: PageSizeOption[];

  /**
   * @description
   * Table state for managing filters and other table state.
   * Can include both controlled state (with setters) and initial state (without setters).
   *
   * @example
   * ```ts
   * // Controlled state -- setGlobalFilter is responsible for updating globalFilter
   * {
   *   globalFilter: 'search-1',
   *   setGlobalFilter: (newValue: string) => {
   *     // handle the new value
   *   },
   * }
   * // Uncontrolled state -- globalFilter is the initial global filter value for the table.
   * // Updates to the state will be handled by the Table component.
   * {
   *   globalFilter: 'search-1',
   * }
   * ```
   *
   * @default undefined
   */
  state: Partial<ControlledTableState> | undefined;

  /**
   * @description
   * Pagination component to render at the bottom of the table for tables that have pagination enabled.
   *
   * @default TablePagination
   */
  pagination: React.ComponentType<TablePaginationProps>;

  /**
   * @description
   * Server-side pagination plugin for handling infinite scroll or "load more" functionality.
   * When provided, enables fetching additional data when reaching the last page.
   *
   * @default undefined
   */
  fetchPlugin?: {
    fetchNextPage: () => void;
    isFetchNextPageInProgress: boolean;
  };

  /**
   * @description
   * If true, enables row selection functionality with checkboxes in the first column.
   * Selection state is ephemeral and resets on component unmount.
   *
   * @default false
   */
  enableRowSelection: boolean;

  /**
   * @description
   * If true, enables sticky column functionality for better horizontal scrolling experience.
   * First and last columns will remain visible during horizontal scroll.
   *
   * @default false
   */
  enableStickySides: boolean;

  /**
   * @description
   * Table body component to render the table rows.
   *
   * @default TableBody
   */
  body: React.ComponentType<TableBodyProps<TableData>>;
}

interface TableOptionalProps {
  /**
   * @description
   * Component to render sub-rows for expandable row functionality.
   * When provided, rows will show expand/collapse controls in the first column.
   * Sub-rows are rendered below the main row content spanning all columns.
   *
   * @default undefined
   */
  subRow?: React.ComponentType<{ row: TableRow<TableData> }>;

  /**
   * @description
   * Component to render in the actions column (last column) for each row.
   * Typically used for row-specific actions like delete buttons, edit links, or dropdown menus.
   * Renders in the last column of body rows only, not affecting the header column configuration.
   *
   * @default undefined
   */
  actions?: React.ComponentType<{ row: TableRow<TableData> }>;
}

/**
 * Input props that users provide to the Table component.
 * Optional props will be filled with defaults via applyDefaultProps.
 */
export interface TableProps<T extends TableData = TableData>
  extends TableRequiredUserProps<T>,
    Partial<TableRequiredFunctionalityProps>,
    TableOptionalProps {}
/**
 * Resolved props with all defaults applied.
 * Child components can rely on these props being defined.
 */
export interface TablePropsResolved<T extends TableData = TableData>
  extends TableRequiredUserProps<T>,
    TableRequiredFunctionalityProps,
    TableOptionalProps {}

/**
 * Represents the possible view states of a table component.
 * These states determine which UI components should be rendered.
 */
export type TableViewState = 'loading' | 'empty' | 'ready' | 'error' | 'filtered-empty';

/**
 * Column filter entry containing column ID and filter value
 */
export type ColumnFilter = {
  id: string;
  value: unknown;
};

export type PaginationState = {
  /** Current page index (0-based) */
  pageIndex: number;
  /** Number of rows per page */
  pageSize: number;
};

export type SortingState = Array<{
  id: string;
  desc: boolean;
}>;

export type ColumnOrderState = string[];

export type ColumnVisibilityState = Record<string, boolean>;

export type RowSelectionState = Record<string, boolean>;

/**
 * Table state containing aspects of table behavior.
 */
export type TableState = {
  /** Global search/filter value */
  globalFilter: string;
  /** Column-specific filter values */
  columnFilters: ColumnFilter[];
  /** Pagination state */
  pagination: PaginationState;
  /** Sorting state */
  sorting: SortingState;
  /** Column order state */
  columnOrder: ColumnOrderState;
  /** Column visibility state */
  columnVisibility: ColumnVisibilityState;
  /** Row selection state */
  rowSelection: RowSelectionState;
};

/**
 * Table state with update functions for controlled state management.
 */
export type ControlledTableState = TableState & {
  setGlobalFilter: (
    updater:
      | TableState['globalFilter']
      | ((old: TableState['globalFilter']) => TableState['globalFilter'])
  ) => void;
  setColumnFilters: (updater: ColumnFilter[] | ((old: ColumnFilter[]) => ColumnFilter[])) => void;
  setPagination: (updater: PaginationState | ((old: PaginationState) => PaginationState)) => void;
  setSorting: (updater: SortingState | ((old: SortingState) => SortingState)) => void;
  setColumnOrder: (
    updater: ColumnOrderState | ((old: ColumnOrderState) => ColumnOrderState)
  ) => void;
  setColumnVisibility: (
    updater: ColumnVisibilityState | ((old: ColumnVisibilityState) => ColumnVisibilityState)
  ) => void;
  setRowSelection: (
    updater: RowSelectionState | ((old: RowSelectionState) => RowSelectionState)
  ) => void;
};
