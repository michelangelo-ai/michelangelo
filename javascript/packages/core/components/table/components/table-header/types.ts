import type {
  ColumnRenderState,
  SortingCapability,
} from '#core/components/table/types/column-types';
import type { TableData } from '#core/components/table/types/data-types';

export type TableHeaderProps<T extends TableData = TableData> = {
  columns: Array<ColumnRenderState<T> & SortingCapability>;
};
