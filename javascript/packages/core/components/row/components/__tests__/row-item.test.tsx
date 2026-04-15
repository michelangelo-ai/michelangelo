import { render, screen } from '@testing-library/react';

import { RowItem } from '../row-item';

import type { CellRenderer } from '#core/components/cell/types';

describe('RowItem', () => {
  it('renders with DefaultCellRenderer when no CellComponent is provided', () => {
    render(
      <RowItem
        item={{ id: 'name', label: 'Name', accessor: 'name' }}
        record={{ name: 'John Doe', age: 30 }}
      />
    );

    expect(screen.getByText('Name')).toBeInTheDocument();
    expect(screen.getByText('John Doe')).toBeInTheDocument();
  });

  it('uses custom CellComponent when provided', () => {
    const CustomCellRenderer: CellRenderer<string> = ({ value }) => (
      <span data-testid="custom-cell">Custom: {value}</span>
    );

    render(
      <RowItem
        item={{ id: 'name', label: 'Name', accessor: 'name' }}
        record={{ name: 'John Doe', age: 30 }}
        CellComponent={CustomCellRenderer}
      />
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

    render(<RowItem item={itemWithAccessor} record={recordWithNestedData} />);

    expect(screen.getByText('Jane Smith')).toBeInTheDocument();
  });
});
