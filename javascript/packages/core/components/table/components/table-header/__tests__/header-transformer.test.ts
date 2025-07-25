import { getTanstackHeaderFixture } from '../__fixtures__/mock-table-header';
import { transformHeaders } from '../header-transformer';

describe('transformHeaders', () => {
  it('transforms TanStack headers to TableHeader format', () => {
    const tanstackHeaders = [
      getTanstackHeaderFixture({ id: 'name', content: 'Name' }),
      getTanstackHeaderFixture({ id: 'age', content: 'Age' }),
    ];

    const result = transformHeaders(tanstackHeaders);

    expect(result).toEqual([
      { id: 'name', content: 'Name' },
      { id: 'age', content: 'Age' },
    ]);
  });

  it('handles empty headers array', () => {
    const result = transformHeaders([]);
    expect(result).toEqual([]);
  });
});
