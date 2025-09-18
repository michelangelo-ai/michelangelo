import { render, screen } from '@testing-library/react';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { RowItem } from '../row-item';

import type { CellRenderer } from '#core/components/cell/types';

describe('RowItem', () => {
  const mockItem = {
    id: 'name',
    label: 'Name',
    accessor: 'name',
  };

  const mockRecord = {
    name: 'John Doe',
    age: 30,
  };

  it('renders with DefaultCellRenderer when no CellComponent is provided', () => {
    render(<RowItem item={mockItem} record={mockRecord} />, buildWrapper([getRouterWrapper()]));

    expect(screen.getByText('Name')).toBeInTheDocument();
    expect(screen.getByText('John Doe')).toBeInTheDocument();
  });

  it('uses custom CellComponent when provided', () => {
    const CustomCellRenderer: CellRenderer<string> = ({ value }) => (
      <span data-testid="custom-cell">Custom: {value}</span>
    );

    render(
      <RowItem item={mockItem} record={mockRecord} CellComponent={CustomCellRenderer} />,
      buildWrapper([getRouterWrapper()])
    );

    expect(screen.getByText('Name')).toBeInTheDocument();
    expect(screen.getByText('Custom: John Doe')).toBeInTheDocument();
  });

  it('uses accessor when provided instead of id for value extraction', () => {
    const itemWithAccessor = {
      id: 'user',
      label: 'User Name',
      accessor: 'profile.name',
    };

    const recordWithNestedData = {
      profile: {
        name: 'Jane Smith',
      },
    };

    render(
      <RowItem item={itemWithAccessor} record={recordWithNestedData} />,
      buildWrapper([getRouterWrapper()])
    );

    expect(screen.getByText('Jane Smith')).toBeInTheDocument();
  });
});
