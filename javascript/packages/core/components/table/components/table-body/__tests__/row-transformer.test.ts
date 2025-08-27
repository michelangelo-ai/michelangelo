import { getTanstackRowFixture } from '../__fixtures__/mock-table-body';
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
          { id: 'row-1-cell-0', content: 'John' },
          { id: 'row-1-cell-1', content: '30' },
        ],
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
          { id: 'row-2-cell-0', content: 'Jane' },
          { id: 'row-2-cell-1', content: '25' },
        ],
        canSelect: true,
        isSelected: false,
        onToggleSelection: expect.any(Function) as (selected: boolean) => void,
        canExpand: true,
        isExpanded: false,
        onToggleExpanded: expect.any(Function) as () => void,
      },
    ]);
  });

  it('handles empty rows array', () => {
    const result = transformRows([]);
    expect(result).toEqual([]);
  });
});
