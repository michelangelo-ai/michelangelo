import type { TableData } from '#core/components/table/types/data-types';

export type TableSelectionContext<T extends TableData = TableData> = {
  selectedRows: T[];
  selectionEnabled: boolean;
  toggleAllRowsSelected: (selected: boolean) => void;
  getIsAllRowsSelected: () => boolean;
  getIsSomeRowsSelected: () => boolean;
};
