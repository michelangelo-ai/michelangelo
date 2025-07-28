import { getTableViewState } from '../get-table-view-state';

describe('getTableViewState', () => {
  const testCases = [
    {
      description: 'returns loading state when loading is true (highest priority)',
      input: { loading: true, dataLength: 10 },
      expected: 'loading',
    },
    {
      description: 'returns empty state when data length is 0 and not loading',
      input: { loading: false, dataLength: 0 },
      expected: 'empty',
    },
    {
      description: 'returns ready state when not loading and data length is greater than 0',
      input: { loading: false, dataLength: 10 },
      expected: 'ready',
    },
  ] as const;

  test.each(testCases)('$description', ({ input, expected }) => {
    const result = getTableViewState(input);
    expect(result).toBe(expected);
  });
});
