import { render, screen } from '@testing-library/react';

import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { TableSortIcon } from '../table-sort-icon';

const mockIcons = {
  sortAscending: () => <div>Sort Ascending Icon</div>,
  sortDescending: () => <div>Sort Descending Icon</div>,
};

describe('TableSortIcon', () => {
  it('renders ascending icon when column is not sorted', () => {
    const column = { getIsSorted: () => false as const };

    render(<TableSortIcon column={column} />, {
      wrapper: getIconProviderWrapper({ icons: mockIcons }),
    });

    expect(screen.getByText('Sort Ascending Icon')).toBeInTheDocument();
  });

  it('renders ascending icon when column is sorted ascending', () => {
    const column = { getIsSorted: () => 'asc' as const };

    render(<TableSortIcon column={column} />, {
      wrapper: getIconProviderWrapper({ icons: mockIcons }),
    });

    expect(screen.getByText('Sort Ascending Icon')).toBeInTheDocument();
  });

  it('renders descending icon when column is sorted descending', () => {
    const column = { getIsSorted: () => 'desc' as const };

    render(<TableSortIcon column={column} />, {
      wrapper: getIconProviderWrapper({ icons: mockIcons }),
    });

    expect(screen.getByText('Sort Descending Icon')).toBeInTheDocument();
  });
});
