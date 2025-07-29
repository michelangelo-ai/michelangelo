import { TableSearchInput } from './components/table-search-input/table-search-input';
import { ActionsContainer, Container, TrailingContentContainer } from './styled-components';

import type { TableActionBarProps } from './types';

export function TableActionBar({
  globalFilter,
  setGlobalFilter,
  configuration,
}: TableActionBarProps) {
  return (
    <Container>
      <ActionsContainer>
        {configuration.enableSearch && (
          <TableSearchInput value={globalFilter} onChange={setGlobalFilter} />
        )}

        {configuration.middle}

        {configuration.trailing && (
          <TrailingContentContainer>{configuration.trailing}</TrailingContentContainer>
        )}
      </ActionsContainer>
    </Container>
  );
}
