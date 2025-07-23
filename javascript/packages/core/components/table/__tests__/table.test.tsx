import { render, screen } from '@testing-library/react';

import { Table } from '../table';

import type { TableColumn } from '../types/column-types';

describe('Table', () => {
  const mockData = [
    { id: 1, name: 'John Doe', age: 30 },
    { id: 2, name: 'Jane Smith', age: 25 },
  ];

  const mockColumns: TableColumn[] = [
    {
      id: 'name',
      label: 'Name',
      accessor: 'name',
    },
    {
      id: 'age',
      label: 'Age',
      accessor: 'age',
    },
  ];

  it('renders table with headers', () => {
    render(<Table data={mockData} columns={mockColumns} />);

    expect(screen.getByText('Name')).toBeInTheDocument();
    expect(screen.getByText('Age')).toBeInTheDocument();
  });

  it('renders table with data rows', () => {
    render(<Table data={mockData} columns={mockColumns} />);

    expect(screen.getByText('John Doe')).toBeInTheDocument();
    expect(screen.getByText('Jane Smith')).toBeInTheDocument();
    expect(screen.getByText('30')).toBeInTheDocument();
    expect(screen.getByText('25')).toBeInTheDocument();
  });

  it('renders empty table when no data provided', () => {
    render(<Table data={[]} columns={mockColumns} />);

    expect(screen.getByText('Name')).toBeInTheDocument();
    expect(screen.getByText('Age')).toBeInTheDocument();
    expect(screen.queryByText('John Doe')).not.toBeInTheDocument();
  });

  it('renders table structure with correct HTML elements', () => {
    render(<Table data={mockData} columns={mockColumns} />);

    const table = screen.getByRole('table');
    expect(table).toBeInTheDocument();

    const headers = screen.getAllByRole('columnheader');
    expect(headers).toHaveLength(2);

    const rows = screen.getAllByRole('row');
    expect(rows).toHaveLength(3); // 1 header row + 2 data rows
  });
});
