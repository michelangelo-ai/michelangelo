import { Signpost } from '#core/components/signpost/signpost';
import { TableStateWrapper } from '../table-state-wrapper';

import type { TableEmptyStateProps } from './types';

export function TableEmptyState({ emptyState }: TableEmptyStateProps) {
  return (
    <TableStateWrapper>
      <Signpost
        illustration={emptyState.icon}
        title={emptyState.title}
        description={emptyState.content}
      />
    </TableStateWrapper>
  );
}
