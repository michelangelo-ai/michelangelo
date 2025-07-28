import type { TableViewState } from '#core/components/table/types/table-types';

export function getTableViewState(input: { loading: boolean; dataLength: number }): TableViewState {
  const { loading, dataLength } = input;

  if (loading) return 'loading';
  if (dataLength === 0) return 'empty';
  return 'ready';
}
