import type { TableViewState } from '#core/components/table/types/table-types';
import type { ApplicationError } from '#core/types/error-types';

export function getTableViewState(input: {
  loading: boolean;
  dataLength: number;
  error: ApplicationError | undefined;
}): TableViewState {
  const { loading, dataLength, error } = input;

  if (loading) return 'loading';
  if (error) return 'error';
  if (dataLength === 0) return 'empty';
  return 'ready';
}
