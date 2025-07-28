import { render, screen } from '@testing-library/react';

import { GrpcStatusCode } from '#core/constants/grpc-status-codes';
import { ApplicationError } from '#core/types/error-types';
import { TableErrorState } from '../table-error-state';

describe('TableErrorState', () => {
  it('should render deadline exceeded error message', () => {
    const error = new ApplicationError('Timeout', GrpcStatusCode.DEADLINE_EXCEEDED);

    render(<TableErrorState error={error} />);

    expect(
      screen.getByRole('row', { name: /It took too long to fulfill the request/ })
    ).toBeInTheDocument();
    expect(screen.getByText(/Try modifying the table filters/)).toBeInTheDocument();
  });

  it('should render invalid argument error message', () => {
    const error = new ApplicationError('Invalid', GrpcStatusCode.INVALID_ARGUMENT);

    render(<TableErrorState error={error} />);

    expect(
      screen.getByRole('row', { name: /Unable to fetch data for the table/ })
    ).toBeInTheDocument();
    expect(screen.getByText(/Try reloading the table/)).toBeInTheDocument();
  });

  it('should render default error message for unknown errors', () => {
    const error = new ApplicationError('Unknown', GrpcStatusCode.UNKNOWN);

    render(<TableErrorState error={error} />);

    expect(
      screen.getByRole('row', { name: /Unable to fetch data for the table/ })
    ).toBeInTheDocument();
    expect(screen.getByText(/Try reloading the table/)).toBeInTheDocument();
  });
});
