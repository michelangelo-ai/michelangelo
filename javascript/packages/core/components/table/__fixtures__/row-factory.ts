import { merge } from 'lodash';
import { vi } from 'vitest';

import type { Cell, Row } from '@tanstack/react-table';
import type { ReactNode } from 'react';
import type { TableData } from '#core/components/table/types/data-types';
import type { DeepPartial } from '#core/types/utility-types';
import type { TableRow } from '../components/table-body/types';

/**
 * Factory for creating internal {@link TableRow} test fixtures.
 * Provides minimal required properties for testing with sensible defaults.
 *
 * @param base - Partial object shared across all test fixtures for a test suite
 * @returns Function that generates a complete table row using overrides.
 *
 * @example
 * ```typescript
 * // Basic usage
 * const buildRow = buildRowFactory();
 * const row = buildRow({ id: 'test-row' });
 *
 * // With base configuration for test suite
 * const buildSelectedRow = buildRowFactory({
 *   isSelected: true,
 *   canSelect: true
 * });
 * const nameRow = buildSelectedRow({
 *   id: 'row-1',
 *   record: { name: 'John Doe' }
 * });
 * ```
 */
export const buildTableRowFactory = <T = unknown>(base: DeepPartial<TableRow<T>> = {}) => {
  return (overrides: DeepPartial<TableRow<T>> = {}): TableRow<T> => {
    const required: TableRow<T> = {
      id: 'test-row',
      cells: [],
      record: {} as T,
      canSelect: false,
      isSelected: false,
      onToggleSelection: () => {
        // Test implementation placeholder
      },
      canExpand: false,
      isExpanded: false,
      onToggleExpanded: () => {
        // Test implementation placeholder
      },
    };

    return merge({}, required, base, overrides);
  };
};

/**
 * Creates a mock TanStack Table Row for testing purposes.
 *
 * @param overrides - Optional overrides for the row.
 * @returns A mock Row object with visible cells and other properties.
 */
export const getTanstackRowFixture = (overrides?: {
  id?: string;
  cellContents?: ReactNode[];
}): Row<TableData> => {
  const id = overrides?.id ?? 'test-row';
  const cellContents = overrides?.cellContents ?? ['Cell 1', 'Cell 2'];

  return {
    id,
    original: { id },
    getVisibleCells: vi.fn(
      () =>
        cellContents.map((content, index) => ({
          id: `${id}-cell-${index}`,
          column: {
            columnDef: {
              cell: content,
              meta: {
                id: `column-${index}`,
                label: `Column ${index + 1}`,
                type: 'string',
              },
            },
          },
          getContext: vi.fn(() => ({})),
          getIsGrouped: vi.fn(() => false),
          getIsAggregated: vi.fn(() => false),
          getIsPlaceholder: vi.fn(() => false),
          getValue: vi.fn(() => content),
        })) as unknown as Cell<TableData, unknown>[]
    ),
    getCanSelect: vi.fn(() => true),
    getIsSelected: vi.fn(() => false),
    toggleSelected: vi.fn(),
    getCanExpand: vi.fn(() => true),
    getIsExpanded: vi.fn(() => false),
    getToggleExpandedHandler: vi.fn(() => vi.fn()),
    toggleExpanded: vi.fn(),
  } as unknown as Row<TableData>;
};
