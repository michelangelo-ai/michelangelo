import { TableFilterOptionList } from '../table-filter-option-list/table-filter-option-list';

import type { TableData } from '#core/components/table/types/data-types';
import type { TableFilterMenuContentProps } from './types';

export function TableFilterMenuContent<T extends TableData = TableData>(
  props: TableFilterMenuContentProps<T>
) {
  const { selectedColumn, filterableColumns, onColumnSelect } = props;

  if (selectedColumn) {
    // TODO: Integrate with our filter factory system using selectedColumn.columnType
    return (
      <div>
        Filter component placeholder for {selectedColumn.id} (type: {selectedColumn.columnType})
      </div>
    );
  }

  return (
    <TableFilterOptionList filterableColumns={filterableColumns} onColumnSelect={onColumnSelect} />
  );
}
