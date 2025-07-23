import type { Cell } from '#core/components/cell/types';
import type { TableData } from './data-types';

export type TableColumn<TData extends TableData = TableData> = Cell<TData>;
