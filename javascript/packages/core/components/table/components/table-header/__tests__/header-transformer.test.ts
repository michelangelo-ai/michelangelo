import { getTanstackHeaderFixture } from '../__fixtures__/mock-table-header';
import { transformHeaders } from '../header-transformer';

describe('transformHeaders', () => {
  it('transforms TanStack headers to TableHeader format', () => {
    const tanstackHeaders = [
      getTanstackHeaderFixture({
        id: 'name',
        content: 'Name',
        canSort: true,
        sortDirection: false,
      }),
      getTanstackHeaderFixture({
        id: 'age',
        content: 'Age',
        canSort: false,
        sortDirection: 'asc',
      }),
    ];

    const result = transformHeaders(tanstackHeaders);

    expect(result).toEqual([
      {
        id: 'name',
        content: 'Name',
        canSort: true,
        onToggleSort: expect.any(Function) as (e: React.MouseEvent<HTMLDivElement>) => void,
        sortDirection: false,
      },
      {
        id: 'age',
        content: 'Age',
        canSort: false,
        onToggleSort: expect.any(Function) as (e: React.MouseEvent<HTMLDivElement>) => void,
        sortDirection: 'asc',
      },
    ]);
  });

  it('handles empty headers array', () => {
    const result = transformHeaders([]);
    expect(result).toEqual([]);
  });
});
