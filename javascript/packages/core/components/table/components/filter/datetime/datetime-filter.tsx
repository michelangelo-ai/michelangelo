import type { ColumnFilterProps } from '../types';

export function DatetimeFilter({ columnId, close }: ColumnFilterProps) {
  // TODO: Implement datetime filter
  return (
    <div style={{ padding: '16px' }}>
      <div>Datetime filter for column: {columnId}</div>
      <button onClick={close}>Close</button>
    </div>
  );
}
