import { useStyletron } from 'baseui';

import { ActiveFilterTag } from './table-active-filter-tag';

import type { ActiveFilterTagListProps } from './types';

export function ActiveFilterTagList<TData = unknown>(props: ActiveFilterTagListProps<TData>) {
  const { filterableColumns, preFilteredRows } = props;
  const [css, theme] = useStyletron();

  const filteredColumns = filterableColumns.filter((column) => {
    const filterValue = column.getFilterValue();
    return filterValue !== undefined && filterValue !== null;
  });

  if (filteredColumns.length === 0) {
    return null;
  }

  return (
    <div className={css({ display: 'flex', flexWrap: 'wrap', gap: theme.sizing.scale300 })}>
      {filteredColumns.map((column) => (
        <ActiveFilterTag key={column.id} column={column} preFilteredRows={preFilteredRows} />
      ))}
    </div>
  );
}
