import { ErrorView } from '#core/components/error-view/error-view';
import { QuestionMark } from '#core/components/illustrations/question-mark/question-mark';
import { TableStateWrapper } from '../table-state-wrapper';

import type { TableNoResultsStateProps } from './types';

export function TableNoResultsState({ clearFilters }: TableNoResultsStateProps) {
  return (
    <TableStateWrapper>
      <ErrorView
        illustration={<QuestionMark />}
        title="There is no information available for selected filters"
        description="Please change or remove filters to see the information."
        buttonConfig={{
          content: 'Clear all filters',
          onClick: clearFilters,
        }}
      />
    </TableStateWrapper>
  );
}
