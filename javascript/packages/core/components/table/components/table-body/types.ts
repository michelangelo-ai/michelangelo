import type { TableData } from '#core/components/table/types/data-types';
import type { TableRow } from '#core/components/table/types/row-types';
import type { WithStickySidesProps } from '../with-sticky-sides/types';

export type TableBodyProps<T extends TableData = TableData> = {
  rows: TableRow<T>[];
  enableRowSelection: boolean;
  subRow?: React.ComponentType<{ row: TableRow<T> }>;
  actions?: React.ComponentType<{ row: TableRow<T> }>;
} & Pick<WithStickySidesProps, 'enableStickySides' | 'scrollRatio'>;
