import { GrpcStatusCode } from '#core/constants/grpc-status-codes';
import { ApplicationError } from '#core/types/error-types';
import { getTableViewState } from '../get-table-view-state';

describe('getTableViewState', () => {
  const testCases = [
    {
      description: 'returns loading state when loading is true (highest priority)',
      input: {
        loading: true,
        dataLength: 10,
        error: undefined,
        hasFiltersApplied: false,
        filteredLength: 0,
      },
      expected: 'loading',
    },
    {
      description: 'returns empty state when data length is 0 and not loading or error',
      input: {
        loading: false,
        dataLength: 0,
        error: undefined,
        hasFiltersApplied: false,
        filteredLength: 0,
      },
      expected: 'empty',
    },
    {
      description: 'returns error state when error exists and not loading',
      input: {
        loading: false,
        dataLength: 10,
        error: new ApplicationError('Test error', GrpcStatusCode.UNKNOWN),
        hasFiltersApplied: false,
        filteredLength: 0,
      },
      expected: 'error',
    },
    {
      description: 'returns empty state when data length is 0 and filters are applied',
      input: {
        loading: false,
        dataLength: 0,
        error: undefined,
        hasFiltersApplied: true,
        filteredLength: 0,
      },
      expected: 'empty',
    },
    {
      description: 'returns ready state when no filters applied and data exists',
      input: {
        loading: false,
        dataLength: 10,
        error: undefined,
        hasFiltersApplied: false,
        filteredLength: 10,
      },
      expected: 'ready',
    },
    {
      description: 'returns ready state when filters applied and data exists',
      input: {
        loading: false,
        dataLength: 10,
        error: undefined,
        hasFiltersApplied: true,
        filteredLength: 10,
      },
      expected: 'ready',
    },
    {
      description: 'returns ready state when filters applied and data exists',
      input: {
        loading: false,
        dataLength: 10,
        error: undefined,
        hasFiltersApplied: true,
        filteredLength: 1,
      },
      expected: 'ready',
    },
    {
      description: 'handles single item datasets',
      input: {
        loading: false,
        dataLength: 1,
        error: undefined,
        hasFiltersApplied: false,
        filteredLength: 1,
      },
      expected: 'ready',
    },
  ] as const;

  test.each(testCases)('$description', ({ input, expected }) => {
    const result = getTableViewState(input);
    expect(result).toBe(expected);
  });
});
