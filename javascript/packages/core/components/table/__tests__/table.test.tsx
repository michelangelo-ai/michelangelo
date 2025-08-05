import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { GrpcStatusCode } from '#core/constants/grpc-status-codes';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getInterpolationProviderWrapper } from '#core/test/wrappers/get-interpolation-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { ApplicationError } from '#core/types/error-types';
import { buildTableColumns, buildTableData } from '../__fixtures__/table-test-helpers';
import { Table } from '../table';

describe('Table', () => {
  describe('with many columns and many rows', () => {
    const numberOfRows = 3;
    const numberOfColumns = 4;

    beforeEach(() => {
      render(
        <Table
          data={buildTableData(numberOfRows, numberOfColumns)}
          columns={buildTableColumns(numberOfColumns)}
        />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );
    });

    it('renders the table', () => {
      expect(screen.getByRole('table')).toBeInTheDocument();
    });

    it('renders column headers', () => {
      expect(screen.getAllByRole('columnheader')).toHaveLength(numberOfColumns);
      expect(
        screen.getByRole('row', { name: 'Column1 Column2 Column3 Column4' })
      ).toBeInTheDocument();
    });

    it('renders data within rows', () => {
      for (const row of [
        'row1-col1-data row1-col2-data row1-col3-data row1-col4-data',
        'row2-col1-data row2-col2-data row2-col3-data row2-col4-data',
        'row3-col1-data row3-col2-data row3-col3-data row3-col4-data',
      ]) {
        expect(screen.getByRole('row', { name: row })).toBeInTheDocument();
      }
    });
  });

  describe('when data is empty', () => {
    const numberOfColumns = 3;

    beforeEach(() => {
      render(<Table data={[]} columns={buildTableColumns(numberOfColumns)} />);
    });

    it('renders the table', () => {
      expect(screen.getByRole('table')).toBeInTheDocument();
    });

    it('renders the empty state', () => {
      expect(screen.getByRole('row', { name: /No data/ })).toBeInTheDocument();
    });

    it('renders column headers', () => {
      expect(screen.getAllByRole('columnheader')).toHaveLength(numberOfColumns);
      expect(screen.getByRole('row', { name: 'Column1 Column2 Column3' })).toBeInTheDocument();
    });
  });

  describe('when data has a single row', () => {
    const numberOfColumns = 4;

    beforeEach(() => {
      render(
        <Table
          data={buildTableData(1, numberOfColumns)}
          columns={buildTableColumns(numberOfColumns)}
        />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );
    });

    it('renders the table', () => {
      expect(screen.getByRole('table')).toBeInTheDocument();
    });

    it('renders column headers', () => {
      expect(screen.getAllByRole('columnheader')).toHaveLength(numberOfColumns);
      expect(
        screen.getByRole('row', { name: 'Column1 Column2 Column3 Column4' })
      ).toBeInTheDocument();
    });

    it('renders data cells', () => {
      expect(
        screen.getByRole('row', {
          name: 'row1-col1-data row1-col2-data row1-col3-data row1-col4-data',
        })
      ).toBeInTheDocument();
    });
  });

  describe('when data has a single column', () => {
    const numberOfRows = 3;

    beforeEach(() => {
      render(
        <Table data={buildTableData(numberOfRows, 1)} columns={buildTableColumns(1)} />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );
    });

    it('renders the table', () => {
      expect(screen.getByRole('table')).toBeInTheDocument();
    });

    it('renders column headers', () => {
      expect(screen.getAllByRole('columnheader')).toHaveLength(1);
      expect(screen.getByRole('row', { name: 'Column1' })).toBeInTheDocument();
    });

    it('renders data within rows', () => {
      for (const row of ['row1-col1-data', 'row2-col1-data', 'row3-col1-data']) {
        expect(screen.getByRole('row', { name: `${row}` })).toBeInTheDocument();
      }
    });
  });

  describe('when loading is true', () => {
    const numberOfRows = 3;
    const numberOfColumns = 4;

    beforeEach(() => {
      render(
        <Table
          data={buildTableData(numberOfRows, numberOfColumns)}
          columns={buildTableColumns(numberOfColumns)}
          loading={true}
        />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );
    });

    it('renders the default loading state', () => {
      expect(screen.getByTestId('table-loading-state')).toBeInTheDocument();
    });

    it('renders column headers when loading', () => {
      expect(screen.getAllByRole('columnheader')).toHaveLength(4);
    });

    it('does not render data rows when loading', () => {
      expect(screen.queryByRole('row', { name: /row/ })).not.toBeInTheDocument();
    });
  });

  describe('when loading with custom loadingView', () => {
    const CustomLoadingView = () => <div data-testid="custom-loading">Custom Loading...</div>;

    beforeEach(() => {
      render(
        <Table
          data={buildTableData(2, 3)}
          columns={buildTableColumns(3)}
          loading={true}
          loadingView={CustomLoadingView}
        />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );
    });

    it('renders the custom loading view', () => {
      expect(screen.getByText('Custom Loading...')).toBeInTheDocument();
    });

    it('does not render the default loading state', () => {
      expect(screen.queryByTestId('table-loading-state')).not.toBeInTheDocument();
    });

    it('renders column headers when loading', () => {
      expect(screen.getAllByRole('columnheader')).toHaveLength(3);
    });

    it('does not render data rows when loading', () => {
      expect(screen.queryByRole('row', { name: /row/ })).not.toBeInTheDocument();
    });
  });

  describe('when error is present', () => {
    beforeEach(() => {
      render(
        <Table
          data={buildTableData(3, 4)}
          columns={buildTableColumns(4)}
          error={new ApplicationError('Test error', GrpcStatusCode.UNKNOWN)}
        />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );
    });

    it('renders error state', () => {
      expect(
        screen.getByRole('row', { name: /Unable to fetch data for the table/ })
      ).toBeInTheDocument();
    });

    it('does not render column headers when error is present', () => {
      expect(screen.queryByRole('columnheader')).not.toBeInTheDocument();
    });

    it('does not render data rows when error is present', () => {
      expect(screen.queryByRole('row', { name: /row/ })).not.toBeInTheDocument();
    });

    it('does not render empty state when error is present', () => {
      expect(screen.queryByText('No data')).not.toBeInTheDocument();
    });
  });

  describe('search functionality integration', () => {
    const testData = [
      { id: '1', name: 'Apple Product', category: 'Electronics' },
      { id: '2', name: 'Banana Split', category: 'Food' },
      { id: '3', name: 'Orange Juice', category: 'Beverage' },
      { id: '4', name: 'Apple Pie', category: 'Food' },
    ];

    const testColumns = [
      { id: 'name', label: 'Name' },
      { id: 'category', label: 'Category' },
    ];

    beforeEach(() => {
      vi.useFakeTimers();
    });

    afterEach(() => {
      vi.runOnlyPendingTimers();
      vi.useRealTimers();
    });

    describe('when search is enabled', () => {
      beforeEach(() => {
        render(
          <Table data={testData} columns={testColumns} actionBarConfig={{ enableSearch: true }} />,
          buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
        );
      });

      it('renders the action bar with search input', () => {
        expect(screen.getByRole('searchbox')).toBeInTheDocument();
      });

      it('renders all data rows initially', () => {
        expect(screen.getAllByRole('row')).toHaveLength(5); // 1 header + 4 rows
      });

      it('filters data when searching', async () => {
        const user = userEvent.setup({
          advanceTimers: vi.advanceTimersByTime.bind(vi) as (ms: number) => void,
        });

        await user.type(screen.getByRole('searchbox'), 'Apple');
        vi.runAllTimers();
        await waitFor(() => {
          expect(screen.getAllByRole('row')).toHaveLength(3); // 1 header + 2 rows
        });

        expect(screen.queryByRole('cell', { name: 'Banana Split' })).not.toBeInTheDocument();
        expect(screen.getByRole('cell', { name: 'Apple Product' })).toBeInTheDocument();
        expect(screen.getByRole('cell', { name: 'Apple Pie' })).toBeInTheDocument();
      });

      it('filters data case-insensitively', async () => {
        const user = userEvent.setup({
          advanceTimers: vi.advanceTimersByTime.bind(vi) as (ms: number) => void,
        });

        await user.type(screen.getByRole('searchbox'), 'apple');
        vi.runAllTimers();
        await waitFor(() => {
          expect(screen.getByRole('cell', { name: 'Apple Product' })).toBeInTheDocument();
          expect(screen.getByRole('cell', { name: 'Apple Pie' })).toBeInTheDocument();
        });
      });

      it('shows filtered empty state when search returns no results', async () => {
        const user = userEvent.setup({
          advanceTimers: vi.advanceTimersByTime.bind(vi) as (ms: number) => void,
        });

        await user.type(screen.getByRole('searchbox'), 'NonExistentItem');
        vi.runAllTimers();
        await waitFor(() => {
          expect(
            screen.getByRole('heading', {
              name: 'There is no information available for selected filters',
            })
          ).toBeInTheDocument();
        });

        expect(screen.getAllByRole('row')).toHaveLength(2); // 1 header + 1 row

        // No data state is not rendered when there are no results
        expect(screen.queryByRole('heading', { name: 'No data' })).not.toBeInTheDocument();
      });

      it('clears search when clear filters button is clicked', async () => {
        const user = userEvent.setup({
          advanceTimers: vi.advanceTimersByTime.bind(vi) as (ms: number) => void,
        });

        await user.type(screen.getByRole('searchbox'), 'NonExistentItem');
        vi.runAllTimers();
        await waitFor(() => {
          expect(
            screen.getByRole('heading', {
              name: 'There is no information available for selected filters',
            })
          ).toBeInTheDocument();
        });

        await user.click(screen.getByRole('button', { name: 'Clear all filters' }));

        await waitFor(() => {
          expect(screen.getAllByRole('row')).toHaveLength(5); // 1 header + 4 rows
        });

        expect(
          screen.queryByRole('heading', {
            name: 'There is no information available for selected filters',
          })
        ).not.toBeInTheDocument();
      });

      it('clears search using input clear button', async () => {
        const user = userEvent.setup({
          advanceTimers: vi.advanceTimersByTime.bind(vi) as (ms: number) => void,
        });

        await user.type(screen.getByRole('searchbox'), 'Apple');
        vi.runAllTimers();
        await waitFor(() => {
          expect(screen.getAllByRole('row')).toHaveLength(3); // 1 header + 2 rows
        });

        await user.click(screen.getByLabelText('Clear value'));

        await waitFor(() => {
          expect(screen.getAllByRole('row')).toHaveLength(5); // 1 header + 4 rows
        });
      });
    });

    describe('when search is disabled', () => {
      beforeEach(() => {
        render(
          <Table data={testData} columns={testColumns} actionBarConfig={{ enableSearch: false }} />,
          buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
        );
      });

      it('does not render the action bar or search input', () => {
        expect(screen.queryByRole('searchbox')).not.toBeInTheDocument();
      });

      it('renders all data rows without filtering', () => {
        expect(screen.getAllByRole('row')).toHaveLength(5); // 1 header + 4 rows
      });
    });

    describe('when search is enabled but data is empty', () => {
      beforeEach(() => {
        render(
          <Table data={[]} columns={testColumns} actionBarConfig={{ enableSearch: true }} />,
          buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
        );
      });

      it('renders search input but shows regular empty state', () => {
        expect(screen.getByRole('searchbox')).toBeInTheDocument();
        expect(screen.getByRole('heading', { name: 'No data' })).toBeInTheDocument();
        expect(
          screen.queryByRole('heading', {
            name: 'There is no information available for selected filters',
          })
        ).not.toBeInTheDocument();
      });

      it('renders empty state when search returns no results', async () => {
        const user = userEvent.setup({
          advanceTimers: vi.advanceTimersByTime.bind(vi) as (ms: number) => void,
        });

        await user.type(screen.getByRole('searchbox'), 'Food');
        vi.runAllTimers();
        await waitFor(() => {
          expect(screen.getByRole('heading', { name: 'No data' })).toBeInTheDocument();
        });

        expect(
          screen.queryByRole('heading', {
            name: 'There is no information available for selected filters',
          })
        ).not.toBeInTheDocument();
      });
    });
  });

  describe('filter menu integration', () => {
    const testData = [
      { id: '1', name: 'Alice Johnson', department: 'Engineering', status: 'Active' },
      { id: '2', name: 'Bob Smith', department: 'Marketing', status: 'Inactive' },
      { id: '3', name: 'Carol Davis', department: 'Engineering', status: 'Active' },
      { id: '4', name: 'David Wilson', department: 'Sales', status: 'Active' },
    ];

    const testColumns = [
      { id: 'name', label: 'Name' },
      { id: 'department', label: 'Department' },
      { id: 'status', label: 'Status' },
    ];

    beforeEach(() => {
      vi.clearAllMocks();
    });

    describe('end-to-end filter menu workflow', () => {
      beforeEach(() => {
        render(
          <Table data={testData} columns={testColumns} actionBarConfig={{ enableFilters: true }} />,
          buildWrapper([
            getBaseProviderWrapper(),
            getInterpolationProviderWrapper(),
            getRouterWrapper(),
          ])
        );
      });

      it('should complete full filter workflow: open menu → select column → apply filter → verify results → close menu', async () => {
        const user = userEvent.setup();

        // Initially should show all 4 rows
        expect(screen.getAllByRole('row')).toHaveLength(5); // 1 header + 4 data rows

        // Step 1: Open filter menu
        const addFilterButton = screen.getByRole('button', { name: 'Add filter' });
        expect(addFilterButton).toBeInTheDocument();
        await user.click(addFilterButton);

        // Step 2: Select department column
        const departmentOption = screen.getByTestId('filter-option-Department');
        await user.click(departmentOption);

        // Step 3: Select Engineering value in categorical filter
        const engineeringCheckbox = screen.getByLabelText('Engineering');
        await user.click(engineeringCheckbox);

        // Step 4: Apply the filter
        const applyButton = screen.getByRole('button', { name: 'Apply' });
        await user.click(applyButton);

        await waitFor(() => {
          const rows = screen.getAllByRole('row');
          expect(rows).toHaveLength(3); // 1 header + 2 Engineering rows
        });

        expect(
          screen.getByRole('row', { name: 'Alice Johnson Engineering Active' })
        ).toBeInTheDocument();
        expect(
          screen.getByRole('row', { name: 'Carol Davis Engineering Active' })
        ).toBeInTheDocument();
        expect(
          screen.queryByRole('row', { name: 'Bob Smith Marketing Inactive' })
        ).not.toBeInTheDocument();
        expect(
          screen.queryByRole('row', { name: 'David Wilson Sales Active' })
        ).not.toBeInTheDocument();
      });

      it('should allow removing filters and show all data again', async () => {
        const user = userEvent.setup();

        // Apply a filter first
        await user.click(screen.getByRole('button', { name: 'Add filter' }));
        await user.click(screen.getByTestId('filter-option-Department'));
        await user.click(screen.getByLabelText('Engineering'));
        await user.click(screen.getByRole('button', { name: 'Apply' }));

        // Verify filter is applied
        await waitFor(() => {
          expect(screen.getAllByRole('row')).toHaveLength(3);
        });

        // Open filter menu again and remove filter
        await user.click(screen.getByRole('button', { name: 'Add filter' }));
        await user.click(screen.getByTestId('filter-option-Department'));
        await user.click(screen.getByLabelText('Engineering')); // Uncheck
        await user.click(screen.getByRole('button', { name: 'Apply' }));

        // Verify all data is shown again
        await waitFor(() => {
          expect(screen.getAllByRole('row')).toHaveLength(5);
        });
      });

      it('should support exclude mode filtering', async () => {
        const user = userEvent.setup();

        // Open filter menu and select Department column
        await user.click(screen.getByRole('button', { name: 'Add filter' }));
        await user.click(screen.getByTestId('filter-option-Department'));

        // Select Marketing (we want to exclude this)
        await user.click(screen.getByLabelText('Marketing'));

        // Enable exclude mode
        await user.click(screen.getByLabelText('Exclude'));
        await user.click(screen.getByRole('button', { name: 'Apply' }));

        // Should show all rows EXCEPT Marketing (Bob Smith)
        await waitFor(() => {
          expect(screen.getAllByRole('row')).toHaveLength(4); // 1 header + 3 non-Marketing rows
        });

        expect(
          screen.getByRole('row', { name: 'Alice Johnson Engineering Active' })
        ).toBeInTheDocument();
        expect(
          screen.getByRole('row', { name: 'Carol Davis Engineering Active' })
        ).toBeInTheDocument();
        expect(screen.getByRole('row', { name: 'David Wilson Sales Active' })).toBeInTheDocument();
        expect(
          screen.queryByRole('row', { name: 'Bob Smith Marketing Inactive' })
        ).not.toBeInTheDocument();
      });
    });
  });

  describe('datetime filter integration', () => {
    const mixedColumns = [
      { id: 'name', label: 'Name' },
      { id: 'createdAt', label: 'Created At', type: 'DATE' },
      { id: 'department', label: 'Department' },
    ];

    const mixedTestData = [
      { id: '1', name: 'Alice Johnson', createdAt: 1672531200, department: 'Engineering' }, // 2023-01-01
      { id: '2', name: 'Bob Smith', createdAt: 1680307200, department: 'Marketing' }, // 2023-04-01
    ];

    beforeEach(() => {
      vi.clearAllMocks();
    });

    it('should open datetime filter for DATE columns', async () => {
      const user = userEvent.setup();

      render(
        <Table
          data={mixedTestData}
          columns={mixedColumns}
          actionBarConfig={{ enableFilters: true }}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      // Open filter menu and select DATE column
      await user.click(screen.getByRole('button', { name: 'Add filter' }));
      await user.click(screen.getByTestId('filter-option-Created At'));

      // Should open datetime filter (not categorical filter)
      // DatetimeFilter should render with Apply button
      expect(screen.getByRole('button', { name: 'Apply' })).toBeInTheDocument();

      // Should not show categorical filter checkboxes
      expect(screen.queryByLabelText('Engineering')).not.toBeInTheDocument();
      expect(screen.queryByLabelText('Marketing')).not.toBeInTheDocument();
    });
  });

  describe('state management integration', () => {
    const testData = [
      { id: '1', name: 'Alice Johnson', department: 'Engineering', status: 'Active' },
      { id: '2', name: 'Bob Smith', department: 'Marketing', status: 'Inactive' },
      { id: '3', name: 'Carol Davis', department: 'Engineering', status: 'Active' },
      { id: '4', name: 'David Wilson', department: 'Sales', status: 'Active' },
    ];

    const testColumns = [
      { id: 'name', label: 'Name' },
      { id: 'department', label: 'Department' },
      { id: 'status', label: 'Status' },
    ];

    beforeEach(() => {
      vi.clearAllMocks();
      vi.useFakeTimers();
    });

    afterEach(() => {
      vi.runOnlyPendingTimers();
      vi.useRealTimers();
    });

    describe('controlled state with search', () => {
      it('respects controlled globalFilter state and updates search UI', () => {
        const controlledState = {
          globalFilter: 'Engineering',
          setGlobalFilter: vi.fn(),
        };

        render(
          <Table
            data={testData}
            columns={testColumns}
            state={controlledState}
            actionBarConfig={{ enableSearch: true }}
          />,
          buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
        );

        expect(screen.getByRole('searchbox')).toHaveValue('Engineering');
        expect(screen.getAllByRole('row')).toHaveLength(3); // 1 header + 2 Engineering rows
      });

      it('updates filtered results when controlled state changes', async () => {
        let currentState = {
          globalFilter: '',
          setGlobalFilter: vi.fn(),
        };
        const TestWrapper = () => (
          <Table
            data={testData}
            columns={testColumns}
            state={currentState}
            actionBarConfig={{ enableSearch: true }}
          />
        );

        const { rerender } = render(
          <TestWrapper />,
          buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
        );

        expect(screen.getAllByRole('row')).toHaveLength(5); // 1 header + 4 data rows
        expect(screen.getByRole('searchbox')).toHaveValue('');

        currentState = {
          globalFilter: 'Marketing',
          setGlobalFilter: vi.fn(),
        };
        rerender(<TestWrapper />);

        // Check that the search input updates
        expect(screen.getByRole('searchbox')).toHaveValue('Marketing');

        await waitFor(() => {
          expect(screen.getAllByRole('row')).toHaveLength(2); // 1 header + 1 Marketing row
        });
      });
    });

    describe('column filters edge cases', () => {
      it('should handle multiple values with OR logic within column', () => {
        render(
          <Table
            data={testData}
            columns={testColumns}
            state={{
              globalFilter: '',
              columnFilters: [{ id: 'department', value: ['Engineering', 'Sales'] }],
            }}
          />,
          buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
        );

        expect(screen.getAllByRole('row')).toHaveLength(4); // 1 header + 3 matching rows
        expect(
          screen.queryByRole('row', { name: 'Bob Smith Marketing Inactive' })
        ).not.toBeInTheDocument();
      });

      it('should combine global filter with column filters', () => {
        render(
          <Table
            data={testData}
            columns={testColumns}
            state={{
              globalFilter: 'Alice',
              columnFilters: [{ id: 'department', value: ['Engineering'] }],
            }}
          />,
          buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
        );

        expect(screen.getAllByRole('row')).toHaveLength(2); // 1 header + 1 matching row
        expect(
          screen.getByRole('row', { name: 'Alice Johnson Engineering Active' })
        ).toBeInTheDocument();
      });

      it('should handle undefined/null filter values gracefully', () => {
        const columnFilters = [
          { id: 'department', value: undefined },
          { id: 'status', value: null },
          { id: 'name', value: [] },
        ];

        render(
          <Table
            data={testData}
            columns={testColumns}
            state={{
              globalFilter: '',
              columnFilters,
            }}
          />,
          buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
        );

        expect(screen.getAllByRole('row')).toHaveLength(5); // 1 header + 4 rows
      });

      it('should use datetime filter for DATE columns and categorical for others', () => {
        const mixedColumns = [
          { id: 'name', label: 'Name', accessor: 'name' },
          { id: 'createdAt', label: 'Created At', accessor: 'createdAt', type: 'DATE' },
          { id: 'department', label: 'Department', accessor: 'department' },
        ];

        const mixedTestData = [
          { name: 'Alice', createdAt: 1672531200, department: 'Engineering' }, // 2023-01-01
          { name: 'Bob', createdAt: 1680307200, department: 'Marketing' }, // 2023-04-01
        ];
        render(
          <Table
            data={mixedTestData}
            columns={mixedColumns}
            state={{
              globalFilter: '',
              columnFilters: [
                {
                  id: 'createdAt',
                  value: {
                    operation: 'RANGE_DATETIME',
                    range: [new Date('2023-01-01'), new Date('2023-03-01')],
                    selection: [],
                    description: 'Q1 2023',
                    exclude: false,
                  },
                },
                { id: 'department', value: ['Engineering'] },
              ],
            }}
          />,
          buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
        );

        expect(screen.getByRole('row', { name: /Alice/ })).toBeInTheDocument();
        expect(screen.queryByRole('row', { name: /Bob/ })).not.toBeInTheDocument();
      });
    });
  });
});
