import { useStyletron } from 'baseui';

import { TableFilterOption } from '../table-filter-option/table-filter-option';
import { ListContainer } from './styled-components';

import type { TableData } from '#core/components/table/types/data-types';
import type { TableFilterOptionListProps } from './types';

export function TableFilterOptionList<T extends TableData = TableData>({
  filterableColumns,
  onColumnSelect,
}: TableFilterOptionListProps<T>) {
  const [css, theme] = useStyletron();

  return (
    <div
      className={css({
        backgroundColor: theme.colors.backgroundPrimary,
        borderRadius: theme.borders.radius300,
        width: '214px',
        display: 'flex',
        flexDirection: 'column',
      })}
    >
      <div
        className={css({
          ...theme.typography.LabelSmall,
          paddingBottom: theme.sizing.scale300,
          paddingTop: theme.sizing.scale500,
          paddingLeft: theme.sizing.scale500,
          paddingRight: theme.sizing.scale500,
        })}
      >
        Select column to filter
      </div>
      <ListContainer>
        {filterableColumns.map((column) => (
          <TableFilterOption
            key={column.id}
            label={column.title}
            onClick={() => onColumnSelect(column)}
          />
        ))}
      </ListContainer>
    </div>
  );
}
