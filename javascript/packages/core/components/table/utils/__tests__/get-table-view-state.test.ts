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
        columnsLength: 5,
      },
      expected: 'loading',
    },
    {
      description: 'returns no-columns state when columns length is 0',
      input: {
        loading: false,
        dataLength: 10,
        error: undefined,
        hasFiltersApplied: false,
        filteredLength: 10,
        columnsLength: 0,
      },
      expected: 'no-columns',
    },
    {
      description: 'returns empty state when data length is 0 and not loading or error',
      input: {
        loading: false,
        dataLength: 0,
        error: undefined,
        hasFiltersApplied: false,
        filteredLength: 0,
        columnsLength: 5,
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
        columnsLength: 5,
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
        columnsLength: 5,
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
        columnsLength: 5,
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
        columnsLength: 5,
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
        columnsLength: 5,
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
        columnsLength: 5,
      },
      expected: 'ready',
    },
    {
      description: 'returns no-columns when columns length is 0 even with data and no error',
      input: {
        loading: false,
        dataLength: 5,
        error: undefined,
        hasFiltersApplied: false,
        filteredLength: 5,
        columnsLength: 0,
      },
      expected: 'no-columns',
    },
    {
      description: 'returns no-columns when both columns and data are empty',
      input: {
        loading: false,
        dataLength: 0,
        error: undefined,
        hasFiltersApplied: false,
        filteredLength: 0,
        columnsLength: 0,
      },
      expected: 'no-columns',
    },
  ] as const;

  test.each(testCases)('$description', ({ input, expected }) => {
    const result = getTableViewState(input);
    expect(result).toBe(expected);
  });
});
