import { useStyletron } from 'baseui';
import { Button, KIND, SIZE } from 'baseui/button';

import { Icon } from '#core/components/icon/icon';
import { getColumnFilter } from '#core/components/table/components/filter/get-column-filter';
import { TableFilterOptionList } from '../table-filter-option-list/table-filter-option-list';

import type { TableData } from '#core/components/table/types/data-types';
import type { TableFilterMenuContentProps } from './types';

export function TableFilterMenuContent<T extends TableData = TableData>(
  props: TableFilterMenuContentProps<T>
) {
  const {
    columnFilters,
    filterableColumns,
    onClose,
    preFilteredRows,
    selectedColumn,
    setColumnFilters,
    setSelectedColumn,
  } = props;

  const [css, theme] = useStyletron();

  if (selectedColumn) {
    const FilterComponent = getColumnFilter(selectedColumn.type);

    return (
      <div className={css({ backgroundColor: theme.colors.backgroundPrimary })}>
        <Button
          onClick={() => setSelectedColumn(undefined)}
          kind={KIND.tertiary}
          size={SIZE.compact}
        >
          <Icon name="arrowLeft" size={18} />
        </Button>
        <FilterComponent
          columnId={selectedColumn.id}
          close={onClose}
          getFilterValue={() => {
            const filter = columnFilters.find((f) => f.id === selectedColumn.id);
            return filter?.value;
          }}
          setFilterValue={(value) => {
            const newFilters = columnFilters.filter((f) => f.id !== selectedColumn.id);
            if (value !== undefined) {
              newFilters.push({ id: selectedColumn.id, value });
            }
            setColumnFilters(newFilters);
          }}
          preFilteredRows={preFilteredRows}
        />
      </div>
    );
  }

  return (
    <TableFilterOptionList
      filterableColumns={filterableColumns}
      setSelectedColumn={setSelectedColumn}
    />
  );
}
