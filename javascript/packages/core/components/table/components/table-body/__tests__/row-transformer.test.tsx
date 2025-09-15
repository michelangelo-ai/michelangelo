import { render, screen } from '@testing-library/react';

import { getTanstackRowFixture } from '#core/components/table/__fixtures__/row-factory';
import { transformRows } from '../row-transformer';

describe('transformRows', () => {
  it('transforms TanStack rows to TableRow format', () => {
    const tanstackRows = [
      getTanstackRowFixture({ id: 'row-1', cellContents: ['John', '30'] }),
      getTanstackRowFixture({ id: 'row-2', cellContents: ['Jane', '25'] }),
    ];

    const result = transformRows(tanstackRows);

    expect(result).toEqual([
      {
        id: 'row-1',
        cells: [
          {
            id: 'row-1-cell-0',
            content: expect.any(Object) as React.ReactElement,
            column: { id: 'column-0', label: 'Column 1', type: 'string' },
            value: 'John',
            isVisible: true,
          },
          {
            id: 'row-1-cell-1',
            content: expect.any(Object) as React.ReactElement,
            column: { id: 'column-1', label: 'Column 2', type: 'string' },
            value: '30',
            isVisible: true,
          },
        ],
        record: { id: 'row-1' },
        canSelect: true,
        isSelected: false,
        onToggleSelection: expect.any(Function) as (selected: boolean) => void,
        canExpand: true,
        isExpanded: false,
        onToggleExpanded: expect.any(Function) as () => void,
      },
      {
        id: 'row-2',
        cells: [
          {
            id: 'row-2-cell-0',
            content: expect.any(Object) as React.ReactElement,
            column: { id: 'column-0', label: 'Column 1', type: 'string' },
            value: 'Jane',
            isVisible: true,
          },
          {
            id: 'row-2-cell-1',
            content: expect.any(Object) as React.ReactElement,
            column: { id: 'column-1', label: 'Column 2', type: 'string' },
            value: '25',
            isVisible: true,
          },
        ],
        record: { id: 'row-2' },
        canSelect: true,
        isSelected: false,
        onToggleSelection: expect.any(Function) as (selected: boolean) => void,
        canExpand: true,
        isExpanded: false,
        onToggleExpanded: expect.any(Function) as () => void,
      },
    ]);

    // Test that cell content renders the expected values
    render(<div>{result[0].cells[0].content}</div>);
    expect(screen.getByText('John')).toBeInTheDocument();

    render(<div>{result[0].cells[1].content}</div>);
    expect(screen.getByText('30')).toBeInTheDocument();

    render(<div>{result[1].cells[0].content}</div>);
    expect(screen.getByText('Jane')).toBeInTheDocument();

    render(<div>{result[1].cells[1].content}</div>);
    expect(screen.getByText('25')).toBeInTheDocument();
  });

  it('handles empty rows array', () => {
    const result = transformRows([]);
    expect(result).toEqual([]);
  });
});
