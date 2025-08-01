import { omit } from 'lodash';

import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { TableData } from '#core/components/table/types/data-types';

/**
 * Resolves the effective column configuration for a specific row based on typeMeta.kind.
 *
 * @param column - The base column configuration which may contain type-specific overrides
 * @param row - The row data containing potential typeMeta.kind information
 * @returns The resolved column configuration to use for this specific row.
 *
 * @example
 * ```ts
 * const column = {
 *   id: 'name',
 *   accessor: 'name',
 *   Draft: {
 *     id: 'name-draft',
 *     accessor: 'spec.content.name',
 *   },
 * };
 *
 * const row = {
 *   typeMeta: { kind: 'Draft' },
 * };
 *
 * const resolvedColumn = resolveColumnForRow(column, row);
 * console.log(resolvedColumn);
 * // { id: 'name-draft', accessor: 'spec.content.name' }
 *
 * const row = {
 *   typeMeta: { kind: 'NotDraft' },
 * };
 *
 * const resolvedColumn = resolveColumnForRow(column, row);
 * console.log(resolvedColumn);
 * // { id: 'name', accessor: 'name' }
 * ```
 */
export function resolveColumnForRow<T extends TableData = TableData>(
  column: ColumnConfig<T>,
  row: T
): ColumnConfig<T> {
  // TODO: #277 generalize typeMeta.kind access in a type-safe way
  // @ts-expect-error - typeMeta may not exist on generic type T, but we handle it safely with optional chaining
  const kind = (row?.typeMeta?.kind as string) ?? ''; // eslint-disable-line @typescript-eslint/no-unsafe-member-access

  // Remove the typeMeta.kind property from the column to avoid infinite recursion
  // if the resolved column is passed to resolveColumnForRow again
  return kind in column ? ({ ...omit(column, kind), ...column[kind] } as ColumnConfig<T>) : column;
}
