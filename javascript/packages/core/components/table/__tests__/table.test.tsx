import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { GrpcStatusCode } from '#core/constants/grpc-status-codes';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { getInterpolationProviderWrapper } from '#core/test/wrappers/get-interpolation-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { ApplicationError } from '#core/types/error-types';
import {
  buildTableColumns,
  buildTableData,
  expectTableHeaders,
  expectTableRows,
} from '../__fixtures__/table-test-helpers';
import { TableBodyProps, TableRow } from '../components/table-body/types';
import { useTableSelectionContext } from '../plugins/selection/table-selection-context';
import { Table } from '../table';

import type { Accessor } from '#core/types/common/studio-types';
import type { TablePaginationProps } from '../components/table-pagination/types';

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
      expectTableHeaders({ dataColumns: numberOfColumns });
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
      expectTableHeaders({ dataColumns: numberOfColumns });
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
      expectTableHeaders({ dataColumns: numberOfColumns });
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
      expectTableHeaders({ dataColumns: 1 });
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
      expectTableHeaders({ dataColumns: 4 });
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
      expectTableHeaders({ dataColumns: 3 });
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

    describe('unFilteredData integration', () => {
      it('uses unFilteredData for filter options in server-side filtering scenarios', async () => {
        const user = userEvent.setup();

        const filteredData = [
          { id: '1', name: 'Alice Johnson', department: 'Engineering', status: 'Active' },
        ];

        const completeData = [
          { id: '1', name: 'Alice Johnson', department: 'Engineering', status: 'Active' },
          { id: '2', name: 'Bob Smith', department: 'Marketing', status: 'Inactive' },
          { id: '3', name: 'Carol Davis', department: 'Sales', status: 'Active' },
        ];

        render(
          <Table
            data={filteredData}
            columns={testColumns}
            unFilteredData={completeData}
            actionBarConfig={{ enableFilters: true }}
          />,
          buildWrapper([
            getBaseProviderWrapper(),
            getInterpolationProviderWrapper(),
            getRouterWrapper(),
          ])
        );

        // Should show only filtered data in table
        expect(screen.getByText('Alice Johnson')).toBeInTheDocument();
        expect(screen.queryByText('Bob Smith')).not.toBeInTheDocument();
        expect(screen.queryByText('Carol Davis')).not.toBeInTheDocument();

        // But filter options should include all departments from unFilteredData
        await user.click(screen.getByRole('button', { name: 'Add filter' }));
        await user.click(screen.getByTestId('filter-option-Department'));

        // Should have all department options available (Engineering, Marketing, Sales)
        expect(screen.getByLabelText('Engineering')).toBeInTheDocument();
        expect(screen.getByLabelText('Marketing')).toBeInTheDocument();
        expect(screen.getByLabelText('Sales')).toBeInTheDocument();
      });

      it('should support building filter options for columns with accessor function', async () => {
        const user = userEvent.setup();

        const data = [
          { id: '1', name: 'Alice Johnson', department: 'Engineering', status: 'Active' },
          { id: '2', name: 'Bob Smith', department: 'Marketing', status: 'Inactive' },
          { id: '3', name: 'Carol Davis', department: 'Sales', status: 'Active' },
        ];

        const columns = [
          { id: 'name', label: 'Name' },
          {
            id: 'department-column-id',
            label: 'Department',
            accessor: ((row: { department: string }) =>
              `Dept: ${row.department}`) as unknown as Accessor<{ department: string }>,
          },
          { id: 'status', label: 'Status' },
        ];

        render(
          <Table data={data} columns={columns} actionBarConfig={{ enableFilters: true }} />,
          buildWrapper([
            getBaseProviderWrapper(),
            getInterpolationProviderWrapper(),
            getRouterWrapper(),
          ])
        );

        await user.click(screen.getByRole('button', { name: 'Add filter' }));
        await user.click(screen.getByTestId('filter-option-Department'));

        await waitFor(() => {
          expect(screen.getAllByText('Dept: Engineering')).toHaveLength(2);
        });
      });

      it('should use first item from multi-cell column for filter options', async () => {
        const user = userEvent.setup();

        const data = [
          {
            id: '1',
            pipelineName: 'my-ml-pipeline',
            revisionId: 'draft-12345',
            description: 'ML training pipeline',
          },
          {
            id: '2',
            pipelineName: 'data-processing',
            revisionId: 'rev-67890',
            description: 'Data preprocessing pipeline',
          },
        ];

        const multiCellColumn = {
          id: 'pipeline-info',
          label: 'Pipeline',
          items: [
            { id: 'pipelineName', accessor: 'pipelineName' },
            { id: 'revisionId', accessor: 'revisionId' },
            { id: 'description', accessor: 'description' },
          ],
        };

        const columns = [multiCellColumn];

        render(
          <Table data={data} columns={columns} actionBarConfig={{ enableFilters: true }} />,
          buildWrapper([
            getBaseProviderWrapper(),
            getInterpolationProviderWrapper(),
            getRouterWrapper(),
          ])
        );

        // Checking table content as baseline for filter options
        expect(screen.getByText('my-ml-pipeline')).toBeInTheDocument();
        expect(screen.getByText('data-processing')).toBeInTheDocument();
        expect(screen.getByText('draft-12345')).toBeInTheDocument();
        expect(screen.getByText('rev-67890')).toBeInTheDocument();
        expect(screen.getByText('ML training pipeline')).toBeInTheDocument();

        await user.click(screen.getByRole('button', { name: 'Add filter' }));
        await user.click(screen.getByTestId('filter-option-Pipeline'));
        await waitFor(() => {
          expect(screen.getAllByText('my-ml-pipeline')).toHaveLength(2);
          expect(screen.getAllByText('data-processing')).toHaveLength(2);
        });

        expect(screen.getAllByText('draft-12345')).toHaveLength(1);
        expect(screen.getAllByText('rev-67890')).toHaveLength(1);
        expect(screen.getAllByText('ML training pipeline')).toHaveLength(1);
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

  describe('pagination integration', () => {
    const buildLargeDataset = (totalRows = 100) => buildTableData(totalRows, 3);
    const testColumns = buildTableColumns(3);

    it('limits rows to page size when pagination is enabled', () => {
      render(
        <Table
          data={buildLargeDataset(100)}
          columns={testColumns}
          pageSizes={[
            { id: 10, label: '10' },
            { id: 25, label: '25' },
          ]}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      // Pagination should limit the displayed rows
      expect(screen.getAllByRole('row')).toHaveLength(11); // header + 10 data rows
      expect(screen.getByText('row1-col1-data')).toBeInTheDocument();
      expect(screen.getByText('row10-col1-data')).toBeInTheDocument();
      expect(screen.queryByText('row11-col1-data')).not.toBeInTheDocument();
    });

    it('hides pagination when disablePagination is true', () => {
      render(
        <Table data={buildLargeDataset(100)} columns={testColumns} disablePagination={true} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      expect(screen.queryByText('Page 1 of')).not.toBeInTheDocument();
      expect(screen.queryByRole('button', { name: /next/i })).not.toBeInTheDocument();
      expect(screen.queryByRole('button', { name: /previous/i })).not.toBeInTheDocument();
    });

    it('shows all rows when pagination is disabled', () => {
      const testData = buildLargeDataset(25);
      render(
        <Table data={testData} columns={testColumns} disablePagination={true} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      expect(screen.getAllByRole('row')).toHaveLength(26);
    });

    it('renders custom pagination component instead of default', () => {
      const CustomPaginationComponent = (props: TablePaginationProps) => (
        <div data-testid="custom-pagination">
          <span>Custom Pagination - Page {props.state.pageIndex + 1}</span>
          <button onClick={() => props.gotoPage(props.state.pageIndex + 1)}>Custom Next</button>
        </div>
      );

      render(
        <Table
          data={buildLargeDataset(50)}
          columns={testColumns}
          pagination={CustomPaginationComponent}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      expect(screen.getByTestId('custom-pagination')).toBeInTheDocument();
      expect(screen.getByText('Custom Pagination - Page 1')).toBeInTheDocument();
      expect(screen.queryByText('Page 1 of')).not.toBeInTheDocument();
    });

    it('respects disablePagination even with custom component', () => {
      const CustomPaginationComponent = (props: TablePaginationProps) => (
        <div data-testid="custom-pagination">
          Custom Pagination - Page {props.state.pageIndex + 1}
        </div>
      );

      render(
        <Table
          data={buildLargeDataset(30)}
          columns={testColumns}
          pagination={CustomPaginationComponent}
          disablePagination={true}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      expect(screen.queryByTestId('custom-pagination')).not.toBeInTheDocument();
      expect(screen.getAllByRole('row')).toHaveLength(31);
    });

    it('shows different data when navigating between pages with custom pagination', async () => {
      const user = userEvent.setup();
      const pagedData = buildTableData(50, 3);

      const CustomPagination = (props: TablePaginationProps) => (
        <div>
          <span>
            Page {props.state.pageIndex + 1} of {props.pageCount}
          </span>
          <button onClick={() => props.gotoPage(props.state.pageIndex + 1)}>Next</button>
        </div>
      );

      render(
        <Table
          data={pagedData}
          columns={testColumns}
          pageSizes={[{ id: 10, label: '10' }]}
          pagination={CustomPagination}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      expect(screen.getByText('row1-col1-data')).toBeInTheDocument();
      expect(screen.queryByText('row11-col1-data')).not.toBeInTheDocument();

      await user.click(screen.getByRole('button', { name: 'Next' }));

      await waitFor(() => {
        expect(screen.getByText('row11-col1-data')).toBeInTheDocument();
        expect(screen.queryByText('row1-col1-data')).not.toBeInTheDocument();
      });
    });

    it('changes page size with custom pagination', async () => {
      const user = userEvent.setup();
      const pagedData = buildLargeDataset(50);

      const CustomPagination = (props: TablePaginationProps) => (
        <div>
          <span>
            Page {props.state.pageIndex + 1} of {props.pageCount}
          </span>
          <select
            value={props.state.pageSize}
            onChange={(e) => props.setPageSize(Number(e.target.value))}
          >
            {props.pageSizes.map((size) => (
              <option key={size.id} value={size.id}>
                {size.label}
              </option>
            ))}
          </select>
        </div>
      );

      render(
        <Table
          data={pagedData}
          columns={testColumns}
          pageSizes={[
            { id: 10, label: '10' },
            { id: 25, label: '25' },
          ]}
          pagination={CustomPagination}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      expect(screen.getByText('Page 1 of 5')).toBeInTheDocument();

      await user.selectOptions(screen.getByRole('combobox'), '25');

      await waitFor(() => {
        expect(screen.getByText('Page 1 of 2')).toBeInTheDocument();
      });
    });

    it('shows pagination state for single page with custom pagination', () => {
      const singlePageData = buildLargeDataset(15);

      const CustomPagination = (props: TablePaginationProps) => (
        <div>
          <span>
            Page {props.state.pageIndex + 1} of {props.pageCount}
          </span>
          <button
            onClick={() => props.gotoPage(props.state.pageIndex + 1)}
            disabled={props.state.pageIndex >= props.pageCount - 1}
          >
            Next
          </button>
        </div>
      );

      render(
        <Table
          data={singlePageData}
          columns={testColumns}
          pageSizes={[{ id: 15, label: '15' }]}
          pagination={CustomPagination}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      expect(screen.getByText('Page 1 of 1')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled();
    });

    it('hides pagination for empty data', () => {
      render(
        <Table data={[]} columns={testColumns} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      expect(screen.queryByText('Page 1 of')).not.toBeInTheDocument();
      expect(screen.queryByRole('button', { name: /next/i })).not.toBeInTheDocument();
      expect(screen.getByText('No data')).toBeInTheDocument();
    });

    it('resets to first page when current page becomes invalid due to filtering', async () => {
      const user = userEvent.setup();

      render(
        <Table
          data={buildLargeDataset(50)}
          columns={testColumns}
          pageSizes={[{ id: 10, label: '10' }]}
          actionBarConfig={{ enableFilters: true }}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      // Verify we start on page 1 of 5
      expect(
        screen.getByRole('button', { name: 'next page. current page 1 of 5' })
      ).toBeInTheDocument();

      // Navigate to page 5 using the next button
      const nextButton = screen.getByRole('button', { name: /next page/i });
      await user.click(nextButton);
      await user.click(nextButton);
      await user.click(nextButton);
      await user.click(nextButton);

      await screen.findByRole('button', { name: 'next page. current page 5 of 5' });

      // Apply column filter that drastically reduces results
      await user.click(screen.getByRole('button', { name: 'Add filter' }));
      await user.click(screen.getByTestId('filter-option-Column1'));
      await user.click(screen.getByLabelText('row1-col1-data'));
      await user.click(screen.getByRole('button', { name: 'Apply' }));

      // Verify table page index is reset to 1 and data is displayed correctly
      await screen.findByRole('button', { name: 'next page. current page 1 of 1' });
      expect(screen.getByText('row1-col1-data')).toBeInTheDocument();
      expect(screen.queryByText('row2-col1-data')).not.toBeInTheDocument();
    });

    it('does not reset page when current page remains valid after filtering', async () => {
      const user = userEvent.setup();

      render(
        <Table
          data={buildLargeDataset(50)}
          columns={testColumns}
          pageSizes={[{ id: 5, label: '5' }]}
          actionBarConfig={{ enableFilters: true }}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      // Navigate to page 2
      const nextButton = screen.getByRole('button', { name: /next page/i });
      await user.click(nextButton);
      await screen.findByRole('button', { name: 'next page. current page 2 of 10' });

      // Apply filter that still leaves 2 pages
      await user.click(screen.getByRole('button', { name: 'Add filter' }));
      await user.click(screen.getByTestId('filter-option-Column1'));
      await user.click(screen.getByLabelText('row5-col1-data'));
      await user.click(screen.getByLabelText('row10-col1-data'));
      await user.click(screen.getByLabelText('row15-col1-data'));
      await user.click(screen.getByLabelText('row20-col1-data'));
      await user.click(screen.getByLabelText('row25-col1-data'));
      await user.click(screen.getByLabelText('row30-col1-data'));
      await user.click(screen.getByLabelText('row35-col1-data'));
      await user.click(screen.getByRole('button', { name: 'Apply' }));

      // Should stay on page 2 since it's still valid (filtered results create 2 pages)
      await screen.findByRole('button', { name: 'next page. current page 2 of 2' });
    });
  });

  describe('sorting functionality', () => {
    const sortableTestData = [
      { id: '1', name: 'Charlie', age: 25, status: 'active' },
      { id: '2', name: 'Alice', age: 30, status: 'inactive' },
      { id: '3', name: 'Bob', age: 20, status: 'active' },
    ];

    const sortableColumns = [
      { id: 'name', label: 'Name', accessor: 'name', enableSorting: true },
      { id: 'age', label: 'Age', accessor: 'age', enableSorting: true },
      { id: 'status', label: 'Status', accessor: 'status', enableSorting: false },
    ];

    const mockIcons = {
      sortAscending: () => <div>Sort Ascending</div>,
      sortDescending: () => <div>Sort Descending</div>,
    };

    it('sorts data when clicking sortable headers', async () => {
      const user = userEvent.setup();

      render(
        <Table data={sortableTestData} columns={sortableColumns} disablePagination={true} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper({ icons: mockIcons }),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      // Initial order: Charlie, Alice, Bob
      expect(screen.getAllByRole('row')[1]).toHaveTextContent('Charlie');

      await user.click(screen.getByRole('columnheader', { name: /name/i }));

      await waitFor(() => {
        expect(screen.getAllByRole('row')[1]).toHaveTextContent('Alice');
        expect(screen.getAllByRole('row')[2]).toHaveTextContent('Bob');
        expect(screen.getAllByRole('row')[3]).toHaveTextContent('Charlie');
      });
    });

    it('toggles sort direction on repeated clicks', async () => {
      const user = userEvent.setup();

      render(
        <Table data={sortableTestData} columns={sortableColumns} disablePagination={true} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper({ icons: mockIcons }),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      const nameHeader = screen.getByRole('columnheader', { name: /name/i });

      // First click: ascending
      await user.click(nameHeader);
      await waitFor(() => {
        expect(screen.getAllByRole('row')[1]).toHaveTextContent('Alice');
      });

      // Second click: descending
      await user.click(nameHeader);
      await waitFor(() => {
        expect(screen.getAllByRole('row')[1]).toHaveTextContent('Charlie');
      });
    });

    it('does not sort when enableSorting is false for column', async () => {
      const user = userEvent.setup();

      render(
        <Table data={sortableTestData} columns={sortableColumns} disablePagination={true} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper({ icons: mockIcons }),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      // Click on non-sortable status column
      await user.click(screen.getByRole('columnheader', { name: /status/i }));

      expect(screen.getAllByRole('row')[1]).toHaveTextContent('Charlie');
      expect(screen.getAllByRole('row')[2]).toHaveTextContent('Alice');
      expect(screen.getAllByRole('row')[3]).toHaveTextContent('Bob');
    });

    it('disables all sorting when disableSorting prop is true', async () => {
      const user = userEvent.setup();

      render(
        <Table
          data={sortableTestData}
          columns={sortableColumns}
          disableSorting={true}
          disablePagination={true}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper({ icons: mockIcons }),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      await user.click(screen.getByRole('columnheader', { name: /name/i }));

      expect(screen.getAllByRole('row')[1]).toHaveTextContent('Charlie');
      expect(screen.getAllByRole('row')[2]).toHaveTextContent('Alice');
      expect(screen.getAllByRole('row')[3]).toHaveTextContent('Bob');
    });

    it('respects initial sorting state', () => {
      render(
        <Table
          data={sortableTestData}
          columns={sortableColumns}
          state={{ sorting: [{ id: 'name', desc: false }] }}
          disablePagination={true}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper({ icons: mockIcons }),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      // Should be sorted alphabetically by name from initial state
      expect(screen.getAllByRole('row')[1]).toHaveTextContent('Alice');
      expect(screen.getAllByRole('row')[2]).toHaveTextContent('Bob');
      expect(screen.getAllByRole('row')[3]).toHaveTextContent('Charlie');
    });
  });

  describe('row selection integration', () => {
    const testData = [
      { id: '1', name: 'Alice Johnson', department: 'Engineering' },
      { id: '2', name: 'Bob Smith', department: 'Marketing' },
      { id: '3', name: 'Carol Davis', department: 'Engineering' },
    ];

    const testColumns = [
      { id: 'name', label: 'Name' },
      { id: 'department', label: 'Department' },
    ];

    it('does not render selection checkboxes when selection is disabled', () => {
      render(
        <Table data={testData} columns={testColumns} state={{ rowSelectionEnabled: false }} />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );

      expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
    });

    it('selects individual rows when row checkbox is clicked after enabling selection', async () => {
      const user = userEvent.setup();

      render(
        <Table data={testData} columns={testColumns} state={{ rowSelectionEnabled: true }} />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );

      const checkboxes = await screen.findAllByRole('checkbox');
      const firstRowCheckbox = checkboxes[1];

      expect(firstRowCheckbox).not.toBeChecked();
      await user.click(firstRowCheckbox);
      expect(firstRowCheckbox).toBeChecked();
    });

    it('selects all rows when header checkbox is clicked after enabling selection', async () => {
      const user = userEvent.setup();

      render(
        <Table data={testData} columns={testColumns} state={{ rowSelectionEnabled: true }} />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );

      const checkboxes = await screen.findAllByRole('checkbox');
      const headerCheckbox = checkboxes[0];

      expect(headerCheckbox).not.toBeChecked();
      await user.click(headerCheckbox);

      checkboxes.forEach((checkbox) => {
        expect(checkbox).toBeChecked();
      });
    });

    it('provides selection context for external components', async () => {
      const user = userEvent.setup();

      const SelectionToggle = () => {
        const { selectionEnabled, setSelectionEnabled } = useTableSelectionContext();
        return (
          <div>
            <span>Selection status: {selectionEnabled ? 'enabled' : 'disabled'}</span>
            <button onClick={() => setSelectionEnabled(!selectionEnabled)}>Toggle Selection</button>
          </div>
        );
      };

      render(
        <Table
          data={testData}
          columns={testColumns}
          actionBarConfig={{
            trailing: <SelectionToggle />,
          }}
        />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );

      expect(screen.getByText('Selection status: disabled')).toBeInTheDocument();
      await user.click(screen.getByRole('button', { name: 'Toggle Selection' }));
      await screen.findByText('Selection status: enabled');
    });

    it('supports enabling selection when starting disabled', async () => {
      const user = userEvent.setup();

      const SelectionToggle = () => {
        const { selectionEnabled, setSelectionEnabled } = useTableSelectionContext();
        return (
          <div>
            <span>Selection status: {selectionEnabled ? 'enabled' : 'disabled'}</span>
            <button onClick={() => setSelectionEnabled(true)}>Enable Selection</button>
          </div>
        );
      };

      render(
        <Table
          data={testData}
          columns={testColumns}
          state={{ rowSelectionEnabled: false }}
          actionBarConfig={{
            trailing: <SelectionToggle />,
          }}
        />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );

      expect(screen.getByText('Selection status: disabled')).toBeInTheDocument();
      await user.click(screen.getByRole('button', { name: 'Enable Selection' }));

      // Selection should now be enabled since setSelectionEnabled(true) was called
      expect(screen.getByText('Selection status: enabled')).toBeInTheDocument();
    });

    it('supports controlled row selection state', async () => {
      const user = userEvent.setup();
      const controlledState = false;
      const mockSetter = vi.fn();

      const SelectionToggle = () => {
        const { selectionEnabled, setSelectionEnabled } = useTableSelectionContext();
        return (
          <div>
            <span>Selection status: {selectionEnabled ? 'enabled' : 'disabled'}</span>
            <button onClick={() => setSelectionEnabled(true)}>Enable Selection</button>
          </div>
        );
      };

      render(
        <Table
          data={testData}
          columns={testColumns}
          state={{
            rowSelectionEnabled: controlledState,
            setRowSelectionEnabled: mockSetter,
          }}
          actionBarConfig={{ trailing: <SelectionToggle /> }}
        />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );

      expect(screen.getByText('Selection status: disabled')).toBeInTheDocument();
      await user.click(screen.getByRole('button', { name: 'Enable Selection' }));
      expect(mockSetter).toHaveBeenCalledWith(true);
    });
  });

  describe('column configuration integration', () => {
    const testData = buildTableData(3, 4);
    const testColumns = buildTableColumns(4);

    beforeEach(() => {
      vi.clearAllMocks();
      localStorage.clear();
    });

    it('renders column configuration button in table header', () => {
      render(
        <Table data={testData} columns={testColumns} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      expect(screen.getByTitle('Configure columns')).toBeInTheDocument();
    });

    it('opens column configuration popover when button is clicked', async () => {
      const user = userEvent.setup();

      render(
        <Table data={testData} columns={testColumns} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      // Column header text is rendered
      expect(screen.getByText('Column1')).toBeInTheDocument();
      expect(screen.getByText('Column2')).toBeInTheDocument();
      expect(screen.getByText('Column3')).toBeInTheDocument();
      expect(screen.getByText('Column4')).toBeInTheDocument();

      await user.click(screen.getByTitle('Configure columns'));

      // First column is the unique identifier column, so it is not rendered in the popover
      expect(screen.getByText('Column1')).toBeInTheDocument();

      // Column headers are rendered twice, once in the table header and once in the popover
      expect(screen.getAllByText('Column2')).toHaveLength(2);
      expect(screen.getAllByText('Column3')).toHaveLength(2);
      expect(screen.getAllByText('Column4')).toHaveLength(2);
    });

    it('reorders columns and reflects changes in table headers', () => {
      render(
        <Table
          data={testData}
          columns={testColumns}
          state={{ columnOrder: ['col2', 'col1', 'col3', 'col4'] }}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      const headers = screen.getAllByRole('columnheader');
      expect(headers[0]).toHaveTextContent('Column2');
      expect(headers[1]).toHaveTextContent('Column1');
      expect(headers[2]).toHaveTextContent('Column3');
      expect(headers[3]).toHaveTextContent('Column4');

      const firstDataRow = screen.getByRole('row', {
        name: 'row1-col2-data row1-col1-data row1-col3-data row1-col4-data',
      });
      expect(firstDataRow).toBeInTheDocument();
    });

    it('hides column and reflects changes in table structure', () => {
      render(
        <Table
          data={testData}
          columns={testColumns}
          state={{ columnVisibility: { col2: false } }}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      // Should only show 3 data columns (Column2 is hidden)
      expectTableHeaders({ dataColumns: 3 });
      expect(screen.getByRole('columnheader', { name: 'Column1' })).toBeInTheDocument();
      expect(screen.queryByRole('columnheader', { name: 'Column2' })).not.toBeInTheDocument();
      expect(screen.getByRole('columnheader', { name: 'Column3' })).toBeInTheDocument();
      expect(screen.getByRole('columnheader', { name: 'Column4' })).toBeInTheDocument();

      // Data rows should exclude the hidden column
      const firstDataRow = screen.getByRole('row', {
        name: 'row1-col1-data row1-col3-data row1-col4-data',
      });
      expect(firstDataRow).toBeInTheDocument();
    });

    it('handles empty columns gracefully', () => {
      render(
        <Table data={[]} columns={[]} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      // Should show empty state when no columns are present
      const emptyState = screen.getByText('No data');
      expect(emptyState).toBeInTheDocument();

      // Should not show configure columns button when no columns exist
      const configButton = screen.queryByTitle('Configure columns');
      expect(configButton).not.toBeInTheDocument();
    });
  });

  describe('tooltip filter integration', () => {
    it('changes displayed table data when tooltip filter is applied', async () => {
      const user = userEvent.setup();
      const testData = [
        { id: '1', name: 'Alice Johnson', department: 'Engineering' },
        { id: '2', name: 'Bob Smith', department: 'Marketing' },
        { id: '3', name: 'Carol Davis', department: 'Engineering' },
      ];

      const columns = [
        {
          id: 'name',
          label: 'Name',
          tooltip: {
            content: 'Click to filter by this name',
            action: 'filter' as const,
          },
        },
        { id: 'department', label: 'Department' },
      ];

      render(
        <Table data={testData} columns={columns} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
          getIconProviderWrapper({
            icons: {
              chevronRight: () => <div>Chevron Right</div>,
            },
          }),
        ])
      );

      expectTableRows({ dataRows: 3 });
      await user.hover(screen.getByText('Alice Johnson'));
      const tooltip = await screen.findByText('Click to filter by this name');
      await user.click(tooltip);

      await waitFor(() => {
        expectTableRows({ dataRows: 1 });
        expect(screen.getByRole('row', { name: 'Alice Johnson Engineering' })).toBeInTheDocument();
      });
    });
  });

  describe('sticky sides integration', () => {
    const testData = buildTableData(3, 4);
    const testColumns = buildTableColumns(4);

    it('applies sticky positioning to columns when enableStickySides is true', () => {
      render(
        <Table data={testData} columns={testColumns} enableStickySides={true} />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );

      // Check for sticky column test IDs that the withStickySides HOC adds
      // Should have sticky cells for both header and data rows in first column
      const stickyCells = screen.getAllByTestId('sticky-cell-left-sticky');
      expect(stickyCells.length).toBeGreaterThan(0);
    });

    it('does not apply sticky positioning when enableStickySides is false', () => {
      render(
        <Table data={testData} columns={testColumns} enableStickySides={false} />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );

      // Should not have sticky cell test IDs
      expect(screen.queryByTestId('sticky-cell-left-sticky')).not.toBeInTheDocument();
    });
  });

  describe('actions column integration', () => {
    const testData = [
      { id: '1', name: 'Alice Johnson', department: 'Engineering' },
      { id: '2', name: 'Bob Smith', department: 'Marketing' },
    ];

    const testColumns = [
      { id: 'name', label: 'Name' },
      { id: 'department', label: 'Department' },
    ];

    const TestActions = ({
      row,
    }: {
      row: TableRow<{ id: string; name: string; department: string }>;
    }) => (
      <div>
        <span>Actions for row {row.record.name}</span>
        <span>Cell count: {row.cells.length}</span>
        <span>First column: {row.cells[0]?.column.label}</span>
        <button>Edit row {row.id}</button>
        <button>Delete row {row.id}</button>
      </div>
    );

    it('renders actions component when actions prop is provided', () => {
      render(
        <Table data={testData} columns={testColumns} actions={TestActions} />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );

      expect(screen.getByText('Actions for row Alice Johnson')).toBeInTheDocument();
      expect(screen.getByText('Actions for row Bob Smith')).toBeInTheDocument();
      expect(screen.getAllByText('Cell count: 2')).toHaveLength(2);
      expect(screen.getAllByText('First column: Name')).toHaveLength(2);
      expect(screen.getByRole('button', { name: 'Edit row 0' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Delete row 0' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Edit row 1' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Delete row 1' })).toBeInTheDocument();
    });
  });

  describe('custom body override integration', () => {
    const testData = [
      { id: '1', name: 'Alice Johnson', department: 'Engineering' },
      { id: '2', name: 'Bob Smith', department: 'Marketing' },
    ];

    const testColumns = [
      { id: 'name', label: 'Name' },
      { id: 'department', label: 'Department' },
    ];

    const CustomTableBody = (props: TableBodyProps) => (
      <tbody>
        <tr>
          <td colSpan={2}>Custom body rendering {props.rows.length} rows</td>
        </tr>
        {props.rows.map((row: TableRow) => (
          <tr key={row.id}>
            <td>Custom: {row.cells[0].content}</td>
            <td>Custom: {row.cells[1].content}</td>
          </tr>
        ))}
      </tbody>
    );

    it('uses custom table body when body prop is provided', () => {
      render(
        <Table data={testData} columns={testColumns} body={CustomTableBody} />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );

      expect(screen.getByText('Custom body rendering 2 rows')).toBeInTheDocument();
      expect(
        screen.getByText((_content, element) => {
          return element?.textContent === 'Custom: Alice Johnson';
        })
      ).toBeInTheDocument();
      expect(
        screen.getByText((_content, element) => {
          return element?.textContent === 'Custom: Bob Smith';
        })
      ).toBeInTheDocument();
    });
  });

  describe('grouping integration', () => {
    const groupedTestData = [
      { id: '1', name: 'Alice Johnson', department: 'Engineering', count: 5 },
      { id: '2', name: 'Bob Smith', department: 'Engineering', count: 3 },
      { id: '3', name: 'Carol Davis', department: 'Marketing', count: 8 },
      { id: '4', name: 'David Wilson', department: 'Marketing', count: 2 },
    ];

    const groupedTestColumns = [
      {
        id: 'department',
        label: 'Department',
        enableGrouping: true,
      },
      { id: 'name', label: 'Name' },
      {
        id: 'count',
        label: 'Count',
        aggregationFn: 'sum' as const,
        aggregatedCell: ({ value }: { value: unknown }) => <span>Total: {String(value)}</span>,
      },
    ];

    const mockIcons = {
      chevronRight: (props: { title: string }) => <div title={props.title}>Chevron Right</div>,
      chevronDown: (props: { title: string }) => <div title={props.title}>Chevron Down</div>,
    };

    it('renders grouped rows with expand controls and aggregated cells when grouping is enabled', () => {
      render(
        <Table
          data={groupedTestData}
          columns={groupedTestColumns}
          state={{ grouping: ['department'] }}
          disablePagination={true}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
          getIconProviderWrapper({ icons: mockIcons }),
        ])
      );

      expectTableRows({ dataRows: 2 });

      // Should render department group headers with aggregated values
      const engineeringRow = screen.getByRole('row', { name: /Engineering.*Total: 8/ });
      const marketingRow = screen.getByRole('row', { name: /Marketing.*Total: 10/ });
      expect(engineeringRow).toBeInTheDocument();
      expect(marketingRow).toBeInTheDocument();

      // Should have expand controls for each grouped row
      expect(within(engineeringRow).getByTitle('Expand')).toBeInTheDocument();
      expect(within(marketingRow).getByTitle('Expand')).toBeInTheDocument();
    });

    it('expands grouped rows to show individual records', async () => {
      const user = userEvent.setup();

      render(
        <Table
          data={groupedTestData}
          columns={groupedTestColumns}
          state={{ grouping: ['department'] }}
          disablePagination={true}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
          getIconProviderWrapper({ icons: mockIcons }),
        ])
      );

      expectTableRows({ dataRows: 2 });
      expect(screen.queryByRole('row', { name: /Alice Johnson/ })).not.toBeInTheDocument();

      // Expand Engineering group
      const engineeringRow = screen.getByRole('row', { name: /Engineering/ });
      const engineeringExpandButton = within(engineeringRow).getByTitle('Expand');
      await user.click(engineeringExpandButton);

      await waitFor(() => {
        expectTableRows({ dataRows: 4 });
      });

      expect(screen.getByRole('row', { name: /Alice Johnson/ })).toBeInTheDocument();
      expect(screen.getByRole('row', { name: /Bob Smith/ })).toBeInTheDocument();

      // Engineering row should now have collapse button
      const updatedEngineeringRow = screen.getByRole('row', { name: /Engineering/ });
      expect(within(updatedEngineeringRow).getByTitle('Collapse')).toBeInTheDocument();

      // Marketing row should still have expand button
      const marketingRow = screen.getByRole('row', { name: /Marketing/ });
      expect(within(marketingRow).getByTitle('Expand')).toBeInTheDocument();
    });

    it('handles custom aggregation functions', () => {
      const customColumns = [
        {
          id: 'department',
          label: 'Department',
          enableGrouping: true,
        },
        { id: 'name', label: 'Name' },
        {
          id: 'count',
          label: 'Average Count',
          aggregationFn: (columnId: string, leafRows: unknown[]) => {
            const sum = leafRows.reduce<number>(
              (acc: number, row: { getValue: (columnId: string) => number }) =>
                acc + row.getValue(columnId),
              0
            );
            const avg = sum / leafRows.length;
            return Math.round(avg * 100) / 100; // Round to 2 decimal places
          },
          aggregatedCell: ({ value }: { value: unknown }) => <span>Avg: {String(value)}</span>,
        },
      ];

      render(
        <Table
          data={groupedTestData}
          columns={customColumns}
          state={{ grouping: ['department'] }}
          disablePagination={true}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      // Should render averaged values in grouped rows
      expect(screen.getByRole('row', { name: /Engineering — Avg: 4/ })).toBeInTheDocument(); // Engineering: (5 + 3) / 2 = 4
      expect(screen.getByRole('row', { name: /Marketing — Avg: 5/ })).toBeInTheDocument(); // Marketing: (8 + 2) / 2 = 5
    });

    it('falls back to regular cell when no aggregatedCell is provided', () => {
      const fallbackColumns = [
        {
          id: 'department',
          label: 'Department',
          enableGrouping: true,
        },
        {
          id: 'count',
          label: 'Count',
          aggregationFn: 'sum' as const,
          // No aggregatedCell provided - should fallback to regular TableCell
        },
      ];

      render(
        <Table
          data={groupedTestData}
          columns={fallbackColumns}
          state={{ grouping: ['department'] }}
          disablePagination={true}
        />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      // Should render aggregated values using fallback TableCell in grouped rows
      expect(screen.getByRole('row', { name: /Engineering 8/ })).toBeInTheDocument(); // Engineering: 5 + 3
      expect(screen.getByRole('row', { name: /Marketing 10/ })).toBeInTheDocument(); // Marketing: 8 + 2
    });
  });

  describe('row expansion integration', () => {
    const testData = [
      { id: '1', name: 'Alice Johnson', department: 'Engineering' },
      { id: '2', name: 'Bob Smith', department: 'Marketing' },
      { id: '3', name: 'Carol Davis', department: 'Engineering' },
    ];

    const testColumns = [
      { id: 'name', label: 'Name' },
      { id: 'department', label: 'Department' },
    ];

    const TestSubRow = ({ row }: { row: TableRow }) => <span>Expanded details for {row.id}</span>;

    const mockIcons = {
      chevronRight: (props: { title: string }) => <div title={props.title}>Chevron Right</div>,
      chevronDown: (props: { title: string }) => <div title={props.title}>Chevron Down</div>,
    };

    it('does not render expand controls when subRow prop is not provided', () => {
      render(
        <Table data={testData} columns={testColumns} />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );

      expect(screen.queryByTitle('Expand')).not.toBeInTheDocument();
    });

    it('renders expand controls and handles expansion workflow', async () => {
      const user = userEvent.setup();

      render(
        <Table data={testData} columns={testColumns} subRow={TestSubRow} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
          getIconProviderWrapper({ icons: mockIcons }),
        ])
      );

      // All rows should have expand controls, no sub-rows initially
      expect(screen.getAllByTitle('Expand')).toHaveLength(3);
      expect(screen.queryByTitle('Collapse')).not.toBeInTheDocument();
      expect(screen.queryByText('Expanded details for 1')).not.toBeInTheDocument();

      // Expand first row
      const firstExpandButton = screen.getAllByTitle('Expand')[0];
      await user.click(firstExpandButton);

      await waitFor(() => {
        expect(screen.getByText('Expanded details for 0')).toBeInTheDocument();
        expect(screen.getByTitle('Collapse')).toBeInTheDocument();
        expect(screen.getAllByTitle('Expand')).toHaveLength(2);
      });

      // Other rows should remain collapsed
      expect(screen.queryByText('Expanded details for 1')).not.toBeInTheDocument();
      expect(screen.queryByText('Expanded details for 2')).not.toBeInTheDocument();

      const collapseButton = screen.getByTitle('Collapse');
      await user.click(collapseButton);

      await waitFor(() => {
        expect(screen.queryByText('Expanded details for 0')).not.toBeInTheDocument();
        expect(screen.getAllByTitle('Expand')).toHaveLength(3);
        expect(screen.queryByTitle('Collapse')).not.toBeInTheDocument();
      });
    });

    it('allows multiple rows to be expanded simultaneously', async () => {
      const user = userEvent.setup();

      render(
        <Table data={testData} columns={testColumns} subRow={TestSubRow} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
          getIconProviderWrapper({ icons: mockIcons }),
        ])
      );

      const expandButtons = screen.getAllByTitle('Expand');

      await user.click(expandButtons[0]);
      await user.click(expandButtons[2]);

      await waitFor(() => {
        expect(screen.getByText('Expanded details for 0')).toBeInTheDocument();
        expect(screen.queryByText('Expanded details for 1')).not.toBeInTheDocument();
        expect(screen.getByText('Expanded details for 2')).toBeInTheDocument();
        expect(screen.getAllByTitle('Expand')).toHaveLength(1);
        expect(screen.getAllByTitle('Collapse')).toHaveLength(2);
      });
    });
  });
});
