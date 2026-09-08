import { render, screen } from '@testing-library/react';

import { interpolate } from '#core/interpolation/interpolate';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { RowItem } from '../row-item';

import type { CellRenderer } from '#core/components/cell/types';

describe('RowItem', () => {
  it('renders with DefaultCellRenderer when no CellComponent is provided', () => {
    render(
      <RowItem
        item={{ id: 'name', label: 'Name', accessor: 'name' }}
        record={{ name: 'John Doe', age: 30 }}
      />,
      buildWrapper([getRouterWrapper()])
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
      />,
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

  it('resolves a function-interpolated url against the record and renders a link', () => {
    const item = {
      id: 'team',
      label: 'Team',
      accessor: 'team.name',
      url: interpolate(({ row }) => (row as { team: { url: string } }).team.url),
    };
    const record = { team: { name: 'Michelangelo', url: 'https://example.com/team' } };

    render(<RowItem item={item} record={record} />, buildWrapper([getRouterWrapper()]));

    const link = screen.getByRole('link', { name: 'Michelangelo' });
    expect(link).toHaveAttribute('href', 'https://example.com/team');
  });
});
