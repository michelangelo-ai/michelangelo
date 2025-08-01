import { EmptyState } from '../components/table-empty-state/types';

import type { ApplicationError } from '#core/types/error-types';
import type { TableActionBarConfig } from '../components/table-action-bar/types';
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
}

/**
 * Input props that users provide to the Table component.
 * Optional props will be filled with defaults via applyDefaultProps.
 */
export interface TableProps<T extends TableData = TableData>
  extends TableRequiredUserProps<T>,
    Partial<TableRequiredFunctionalityProps> {}
/**
 * Resolved props with all defaults applied.
 * Child components can rely on these props being defined.
 */
export interface TablePropsResolved<T extends TableData = TableData>
  extends TableRequiredUserProps<T>,
    TableRequiredFunctionalityProps {}

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

/**
 * Table state containing aspects of table behavior.
 */
export type TableState = {
  /** Global search/filter value */
  globalFilter: string;
  /** Column-specific filter values */
  columnFilters: ColumnFilter[];
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
};
