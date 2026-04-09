import { ActiveFilterTagList } from './components/table-active-filter-tag-list/table-active-filter-tag-list';
import { TableFilterMenu } from './components/table-filter-menu/table-filter-menu';
import { TableSearchInput } from './components/table-search-input/table-search-input';
import { ShareTableUrlButton } from '../share-table-url-button/share-table-url-button';
import { ActionsContainer, Container, TrailingContentContainer } from './styled-components';

import type { TableActionBarProps } from './types';

export function TableActionBar<T>({
  globalFilter,
  setGlobalFilter,
  columnFilters,
  setColumnFilters,
  preFilteredRows,
  configuration,
  filterableColumns = [],
  getShareUrl,
}: TableActionBarProps<T>) {
  return (
    <Container>
      <ActionsContainer>
        {configuration.enableSearch && (
          <TableSearchInput value={globalFilter} onChange={setGlobalFilter} />
        )}

        {configuration.enableFilters && filterableColumns.length > 0 && (
          <TableFilterMenu
            filterableColumns={filterableColumns}
            columnFilters={columnFilters}
            setColumnFilters={setColumnFilters}
            preFilteredRows={preFilteredRows}
          />
        )}

        {configuration.middle}

        {(configuration.trailing || (configuration.enableShareUrl && getShareUrl)) && (
          <TrailingContentContainer>
            {configuration.trailing}
            {configuration.enableShareUrl && getShareUrl && (
              <ShareTableUrlButton
                buildShareUrl={getShareUrl}
                currentState={{
                  globalFilter,
                  columnFilters,
                }}
              />
            )}
          </TrailingContentContainer>
        )}
      </ActionsContainer>

      {configuration.enableFilters && filterableColumns.length > 0 && (
        <ActiveFilterTagList
          filterableColumns={filterableColumns}
          preFilteredRows={preFilteredRows}
        />
      )}
    </Container>
  );
}
