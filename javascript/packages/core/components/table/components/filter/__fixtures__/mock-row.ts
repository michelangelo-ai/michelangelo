import type { Row } from '@tanstack/react-table';

/**
 * Creates a mock TanStack Table Row for testing filter functions.
 *
 * @param data - The row data object
 * @returns A mock Row object with getValue method and original data
 */
export function createMockRow<T extends Record<string, unknown>>(data: T): Row<T> {
  return {
    getValue: (columnId: string) => data[columnId],
    original: data,
  } as unknown as Row<T>;
}
