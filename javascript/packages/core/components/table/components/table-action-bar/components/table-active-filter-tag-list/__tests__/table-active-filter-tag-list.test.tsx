import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { CellType } from '#core/components/cell/constants';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { ActiveFilterTagList } from '../table-active-filter-tag-list';

import type { FilterableColumn } from '#core/components/table/components/table-action-bar/types';

describe('ActiveFilterTagList', () => {
  const mockFilterableColumns: FilterableColumn[] = [
    {
      id: 'testCol1',
      label: 'Option 1',
      type: CellType.TEXT,
      getFilterValue: () => ['opt1', 'opt2', 'opt3'],
      setFilterValue: vi.fn(),
    },
    {
      id: 'testCol2',
      label: 'Option 2',
      type: CellType.DATE,
      getFilterValue: () => ({
        range: ['dummy'],
        selection: ['dummy'],
        description: 'dateOption',
      }),
      setFilterValue: vi.fn(),
    },
    {
      id: 'testCol3',
      label: 'Option 3',
      type: CellType.TEXT,
      getFilterValue: () => undefined,
      setFilterValue: vi.fn(),
    },
  ];

  test('renders nothing when no columns have active filters', () => {
    const columnsWithoutFilters = mockFilterableColumns.map((col) => ({
      ...col,
      getFilterValue: () => undefined,
    }));

    render(
      <ActiveFilterTagList filterableColumns={columnsWithoutFilters} preFilteredRows={[]} />,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.queryByText('Option 1')).not.toBeInTheDocument();
    expect(screen.queryByText('Option 2')).not.toBeInTheDocument();
    expect(screen.queryByText('Option 3')).not.toBeInTheDocument();
  });

  test('renders active filter tags for columns with filter values', () => {
    render(
      <ActiveFilterTagList filterableColumns={mockFilterableColumns} preFilteredRows={[]} />,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByText('(3) Option 1: opt1, opt2, opt3')).toBeInTheDocument();
    expect(screen.getByText('Option 2: dateOption')).toBeInTheDocument();
    expect(screen.queryByText('Option 3')).not.toBeInTheDocument();
  });

  test('clicking delete button calls setFilterValue with undefined', async () => {
    const user = userEvent.setup();
    const setFilterMock = vi.fn();
    const columnsWithMock = [
      {
        ...mockFilterableColumns[0],
        setFilterValue: setFilterMock,
      },
    ];

    render(
      <ActiveFilterTagList filterableColumns={columnsWithMock} preFilteredRows={[]} />,
      buildWrapper([getBaseProviderWrapper()])
    );

    const deleteButton = screen.getAllByTitle('Delete')[0];
    await user.click(deleteButton);

    expect(setFilterMock).toHaveBeenCalledWith(undefined);
  });

  test('clicking on tag opens filter popover', async () => {
    const user = userEvent.setup();

    render(
      <ActiveFilterTagList filterableColumns={mockFilterableColumns} preFilteredRows={[]} />,
      buildWrapper([getBaseProviderWrapper()])
    );

    const tag = screen.getByText('(3) Option 1: opt1, opt2, opt3');
    await user.click(tag);

    // Should open the categorical filter (since it's TEXT type)
    expect(screen.getByRole('checkbox', { name: 'Select All' })).toBeInTheDocument();
  });
});
