import { ErrorView } from '#core/components/error-view/error-view';
import { TableStateWrapper } from '../table-state-wrapper';

import type { TableEmptyStateProps } from './types';

export function TableEmptyState({ emptyState }: TableEmptyStateProps) {
  return (
    <TableStateWrapper>
      <ErrorView
        illustration={emptyState.icon}
        title={emptyState.title}
        description={emptyState.content}
      />
    </TableStateWrapper>
  );
}
