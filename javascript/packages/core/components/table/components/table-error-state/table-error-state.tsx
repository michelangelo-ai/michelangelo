import { ErrorView } from '#core/components/error-view/error-view';
import { CircleExclamationMark } from '#core/components/illustrations/circle-exclamation-mark/circle-exclamation-mark';
import { GrpcStatusCode } from '#core/constants/grpc-status-codes';
import { TableStateWrapper } from '../table-state-wrapper';

import type { TableErrorStateProps } from './types';

export function TableErrorState({ error }: TableErrorStateProps) {
  switch (error.code as GrpcStatusCode) {
    case GrpcStatusCode.DEADLINE_EXCEEDED:
      return (
        <TableStateWrapper>
          <ErrorView
            illustration={<CircleExclamationMark height="64px" width="64px" />}
            title="It took too long to fulfill the request"
            description="Try modifying the table filters to reduce the request size."
          />
        </TableStateWrapper>
      );

    case GrpcStatusCode.INVALID_ARGUMENT:
    default:
      return (
        <TableStateWrapper>
          <ErrorView
            illustration={<CircleExclamationMark height="64px" width="64px" />}
            title="Unable to fetch data for the table"
            description="Try reloading the table and retrying the request."
          />
        </TableStateWrapper>
      );
  }
}
