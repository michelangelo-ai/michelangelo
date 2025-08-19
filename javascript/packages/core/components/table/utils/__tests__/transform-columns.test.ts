import { getTanstackColumn } from '../__fixtures__/tanstack-builders';
import { transformColumns } from '../transform-columns';

describe('transformColumns', () => {
  it('includes sorting capabilities', () => {
    const tanstackColumns = [
      getTanstackColumn({
        columnConfig: {
          id: 'name',
          label: 'Name',
        },
        canSort: true,
        sortDirection: false,
      }),
      getTanstackColumn({
        columnConfig: {
          id: 'age',
          label: 'Age',
        },
        canSort: false,
        sortDirection: 'asc',
      }),
    ];

    const result = transformColumns(tanstackColumns);

    expect(result).toEqual([
      expect.objectContaining({
        id: 'name',
        label: 'Name',
        canSort: true,
        onToggleSort: expect.any(Function) as (e: React.MouseEvent<HTMLDivElement>) => void,
        sortDirection: false,
      }),
      expect.objectContaining({
        id: 'age',
        label: 'Age',
        canSort: false,
        onToggleSort: expect.any(Function) as (e: React.MouseEvent<HTMLDivElement>) => void,
        sortDirection: 'asc',
      }),
    ]);
  });

  it('includes filtering capabilities', () => {
    const tanstackColumns = [
      getTanstackColumn({
        columnConfig: {
          id: 'name',
          label: 'Name',
        },
        canFilter: true,
        filterValue: 'test',
      }),
    ];

    const result = transformColumns(tanstackColumns);

    expect(result).toEqual([
      expect.objectContaining({
        id: 'name',
        label: 'Name',
        canFilter: true,
        getFilterValue: expect.any(Function) as () => string,
        setFilterValue: expect.any(Function) as (value: string) => void,
      }),
    ]);

    expect(tanstackColumns[0].getFilterValue()).toBe('test');
  });

  it('handles empty headers array', () => {
    const result = transformColumns([]);
    expect(result).toEqual([]);
  });
});
