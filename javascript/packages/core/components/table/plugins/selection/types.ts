import type { TableData } from '#core/components/table/types/data-types';
import type { TableRow } from '#core/components/table/types/row-types';

export type TableSelectionContext<T extends TableData = TableData> = {
  selectedRows: TableRow<T>[];
  selectionEnabled: boolean;
  setSelectionEnabled: (enabled: boolean) => void;
  toggleAllRowsSelected: (selected: boolean) => void;
  getIsAllRowsSelected: () => boolean;
  getIsSomeRowsSelected: () => boolean;
};
