import { GrpcStatusCode } from '#core/constants/grpc-status-codes';
import { ApplicationError } from '#core/types/error-types';
import { getTableViewState } from '../get-table-view-state';

describe('getTableViewState', () => {
  const testCases = [
    {
      description: 'returns loading state when loading is true (highest priority)',
      input: { loading: true, dataLength: 10, error: undefined },
      expected: 'loading',
    },
    {
      description: 'returns empty state when data length is 0 and not loading or error',
      input: { loading: false, dataLength: 0, error: undefined },
      expected: 'empty',
    },
    {
      description: 'returns error state when error exists and not loading',
      input: {
        loading: false,
        dataLength: 10,
        error: new ApplicationError('Test error', GrpcStatusCode.UNKNOWN),
      },
      expected: 'error',
    },
    {
      description:
        'returns ready state when not loading, data length is greater than 0, and no error',
      input: { loading: false, dataLength: 10, error: undefined },
      expected: 'ready',
    },
  ] as const;

  test.each(testCases)('$description', ({ input, expected }) => {
    const result = getTableViewState(input);
    expect(result).toBe(expected);
  });
});
