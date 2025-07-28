import { EmptyState } from '../components/table-empty-state/types';

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
export type TableViewState = 'loading' | 'empty' | 'ready';
