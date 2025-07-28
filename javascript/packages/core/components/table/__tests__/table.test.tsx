import { render, screen } from '@testing-library/react';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getInterpolationProviderWrapper } from '#core/test/wrappers/get-interpolation-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
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

    it('renders column headers', () => {
      expect(screen.getAllByRole('columnheader')).toHaveLength(numberOfColumns);
      expect(screen.getByRole('row', { name: 'Column1 Column2 Column3' })).toBeInTheDocument();
    });

    it('renders no data cells', () => {
      expect(screen.queryAllByRole('cell')).toHaveLength(0);
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

    it('does not render column headers when loading', () => {
      expect(screen.queryByRole('columnheader')).not.toBeInTheDocument();
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

    it('does not render column headers when loading', () => {
      expect(screen.queryByRole('columnheader')).not.toBeInTheDocument();
    });

    it('does not render data rows when loading', () => {
      expect(screen.queryByRole('row', { name: /row/ })).not.toBeInTheDocument();
    });
  });
});
