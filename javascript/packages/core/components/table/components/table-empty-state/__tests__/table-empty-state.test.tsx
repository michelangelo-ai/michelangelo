import { render, screen } from '@testing-library/react';

import { CircleExclamationMark } from '#core/components/illustrations/circle-exclamation-mark/circle-exclamation-mark';
import { TableEmptyState } from '../table-empty-state';

describe('TableEmptyState', () => {
  it('should render with title only', () => {
    const emptyState = {
      title: 'No data available',
    };

    render(<TableEmptyState emptyState={emptyState} />);

    expect(screen.getByRole('heading', { name: 'No data available' })).toBeInTheDocument();
  });

  it('should render with title and content', () => {
    const emptyState = {
      title: 'No results found',
      content: 'Try adjusting your filters to see more data.',
    };

    render(<TableEmptyState emptyState={emptyState} />);

    expect(screen.getByRole('heading', { name: 'No results found' })).toBeInTheDocument();
    expect(screen.getByText('Try adjusting your filters to see more data.')).toBeInTheDocument();
  });

  it('should render with title and icon', () => {
    const emptyState = {
      title: 'Empty table',
      icon: (
        <div data-testid="empty-icon">
          <CircleExclamationMark height="64px" width="64px" />
        </div>
      ),
    };

    render(<TableEmptyState emptyState={emptyState} />);

    expect(screen.getByRole('heading', { name: 'Empty table' })).toBeInTheDocument();
    expect(screen.getByTestId('empty-icon')).toBeInTheDocument();
  });

  it('should render with all props provided', () => {
    const emptyState = {
      title: 'No data',
      content: 'No data is present.',
      icon: (
        <div data-testid="complete-icon">
          <CircleExclamationMark height="64px" width="64px" />
        </div>
      ),
    };

    render(<TableEmptyState emptyState={emptyState} />);

    expect(screen.getByRole('heading', { name: 'No data' })).toBeInTheDocument();
    expect(screen.getByText('No data is present.')).toBeInTheDocument();
    expect(screen.getByTestId('complete-icon')).toBeInTheDocument();
  });
});
