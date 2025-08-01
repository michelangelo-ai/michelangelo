import { TableFilterMenu } from './components/table-filter-menu/table-filter-menu';
import { TableSearchInput } from './components/table-search-input/table-search-input';
import { ActionsContainer, Container, TrailingContentContainer } from './styled-components';

import type { TableActionBarProps } from './types';

export function TableActionBar<T>({
  globalFilter,
  setGlobalFilter,
  configuration,
  filterableColumns = [],
}: TableActionBarProps<T>) {
  return (
    <Container>
      <ActionsContainer>
        {configuration.enableSearch && (
          <TableSearchInput value={globalFilter} onChange={setGlobalFilter} />
        )}

        {configuration.enableFilters && filterableColumns.length > 0 && (
          <TableFilterMenu filterableColumns={filterableColumns} />
        )}

        {configuration.middle}

        {configuration.trailing && (
          <TrailingContentContainer>{configuration.trailing}</TrailingContentContainer>
        )}
      </ActionsContainer>
    </Container>
  );
}
