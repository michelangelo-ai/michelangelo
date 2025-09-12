import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { buildColumnFactory } from '#core/components/table/__fixtures__/column-factory';
import { buildTableRowFactory } from '#core/components/table/__fixtures__/row-factory';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { getInterpolationProviderWrapper } from '#core/test/wrappers/get-interpolation-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { TableCell } from '../table-cell';

import type { ColumnConfig } from '../../../types/column-types';
import type { ColumnTooltipContentRendererProps } from '../types';

describe('TableCell', () => {
  const buildRow = buildTableRowFactory<unknown>({ id: 'row-1', record: { name: 'John Doe' } });

  it('should render basic text cell', () => {
    const column: ColumnConfig = {
      id: 'name',
      label: 'Name',
      type: 'text',
    };

    render(
      <TableCell column={column} record={{ name: 'John Doe' }} value="John Doe" row={buildRow()} />,
      buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
    );

    expect(screen.getByText('John Doe')).toBeInTheDocument();
  });

  it('should resolve interpolations in column config', () => {
    const column: ColumnConfig = {
      id: 'name',
      label: 'label',
      type: 'text',
      url: 'https://${row.name}.com',
    };

    const record = { name: 'Jane Smith' };

    render(
      <TableCell column={column} record={record} value="Jane Smith" row={buildRow({ record })} />,
      buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
    );

    expect(screen.getByRole('link', { name: 'Jane Smith' })).toHaveAttribute(
      'href',
      'https://Jane Smith.com'
    );
  });

  describe('tooltip functionality', () => {
    const mockSetColumnFilterValue = vi.fn();
    const buildColumn = buildColumnFactory();

    const defaultProps = {
      record: { id: 1, name: 'Test Record' },
      value: 'test-value',
      columnFilterValue: undefined,
      setColumnFilterValue: mockSetColumnFilterValue,
      row: buildRow({ record: { id: 1, name: 'Test Record' } }),
    };

    beforeEach(() => {
      mockSetColumnFilterValue.mockClear();
    });

    it('renders cell without tooltip when column has no tooltip config', () => {
      const column = buildColumn({ id: 'basic-column', label: 'Basic Column' });

      render(
        <TableCell {...defaultProps} column={column} />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );

      expect(screen.getByText('test-value')).toBeInTheDocument();
      expect(screen.queryByTestId('tooltip-hover-container')).not.toBeInTheDocument();
    });

    it('renders cell with tooltip when column has tooltip config', () => {
      const column = buildColumn({
        id: 'tooltip-column',
        label: 'Tooltip Column',
        tooltip: {
          content: 'Click to filter by this value',
          action: 'filter',
        },
      });

      render(
        <TableCell {...defaultProps} column={column} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      expect(screen.getByText('test-value')).toBeInTheDocument();
      expect(screen.getByTestId('tooltip-hover-container')).toBeInTheDocument();
    });

    it('shows tooltip content on hover', async () => {
      const user = userEvent.setup();
      const column = buildColumn({
        tooltip: {
          content: 'Click to filter by this value',
          action: 'filter',
        },
      });

      render(
        <TableCell {...defaultProps} column={column} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      await user.hover(screen.getByText('test-value'));
      await screen.findByText('Click to filter by this value');
    });

    it('applies filter when tooltip is clicked', async () => {
      const user = userEvent.setup();
      const column = buildColumn({
        id: 'filter-column',
        tooltip: {
          content: 'Click to filter by this value',
          action: 'filter',
        },
      });

      render(
        <TableCell {...defaultProps} column={column} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getIconProviderWrapper({
            icons: {
              chevronRight: () => <div>Chevron Right</div>,
            },
          }),
          getRouterWrapper(),
        ])
      );

      await user.hover(screen.getByText('test-value'));
      const tooltipContent = await screen.findByText('Click to filter by this value');
      await user.click(tooltipContent);

      expect(mockSetColumnFilterValue).toHaveBeenCalledWith(['test-value']);
    });

    it('hides tooltip when same filter is already applied', () => {
      const column = buildColumn({
        id: 'existing-filter-column',
        tooltip: {
          content: 'Click to filter by this value',
          action: 'filter',
        },
      });

      const propsWithExistingFilter = {
        ...defaultProps,
        columnFilterValue: ['test-value'],
      };

      render(
        <TableCell {...propsWithExistingFilter} column={column} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      expect(screen.getByText('test-value')).toBeInTheDocument();
      expect(screen.queryByTestId('tooltip-hover-container')).not.toBeInTheDocument();
    });

    it('renders custom tooltip content with access to row cells', async () => {
      const user = userEvent.setup();
      const CustomTooltipContent = (props: ColumnTooltipContentRendererProps<unknown>) => (
        <div>
          <div>Row contains {props.row?.cells?.length ?? 0} cells</div>
          <div>Cell IDs: {props.row?.cells?.map((cell) => cell.id).join(', ') ?? ''}</div>
        </div>
      );

      const column = buildColumn({
        id: 'custom-tooltip-column',
        tooltip: {
          content: CustomTooltipContent,
          action: 'filter',
        },
      });

      const mockRow = buildRow({
        id: 'test-row',
        cells: [
          {
            id: 'cell-1',
            content: <div>Content 1</div>,
            column: { id: 'col-1', label: 'Col 1' },
            value: 'value1',
          },
          {
            id: 'cell-2',
            content: <div>Content 2</div>,
            column: { id: 'col-2', label: 'Col 2' },
            value: 'value2',
          },
        ],
        record: defaultProps.record,
      });

      render(
        <TableCell {...defaultProps} column={column} row={mockRow} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getInterpolationProviderWrapper(),
          getRouterWrapper(),
        ])
      );

      await user.hover(screen.getByText('test-value'));
      await screen.findByText('Row contains 2 cells');
      await screen.findByText('Cell IDs: cell-1, cell-2');
    });
  });
});
