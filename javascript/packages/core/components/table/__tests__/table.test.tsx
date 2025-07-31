import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { GrpcStatusCode } from '#core/constants/grpc-status-codes';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
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
  });
});
