import type { TableColumn } from './column-types';
import type { TableData } from './data-types';

export interface TableProps<T extends TableData = TableData> {
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
  columns: TableColumn<T>[];
}
