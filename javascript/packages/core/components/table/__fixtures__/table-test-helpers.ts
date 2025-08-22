import { screen } from '@testing-library/react';
import { fromPairs } from 'lodash';

import type { ColumnConfig } from '../types/column-types';

export function buildTableData(
  numberOfRows: number,
  numberOfColumns: number
): Record<string, string>[] {
  return getArrayWithLength(numberOfRows).map((rowId) =>
    fromPairs(
      getArrayWithLength(numberOfColumns).map((columnId) => [
        `col${columnId}`,
        `row${rowId}-col${columnId}-data`,
      ])
    )
  );
}

export function buildTableColumns(numberOfColumns: number): ColumnConfig[] {
  return getArrayWithLength(numberOfColumns).map((columnId) => ({
    id: `col${columnId}`,
    label: `Column${columnId}`,
    accessor: `col${columnId}`,
  }));
}

export function getArrayWithLength(length: number): number[] {
  return Array.from({ length }, (_, i) => i + 1);
}

/**
 * Helper to assert expected column header count, accounting for table structure.
 * By default includes the column configuration button.
 */
export function expectTableHeaders(options: { dataColumns: number; hasConfigButton?: boolean }) {
  let expectedCount = options.dataColumns;
  if (options.hasConfigButton !== false) expectedCount += 1; // defaults to true

  expect(screen.getAllByRole('columnheader')).toHaveLength(expectedCount);
}

/**
 * Helper to assert expected row count, accounting for table structure.
 */
export function expectTableRows(options: { dataRows: number }) {
  const expectedCount = options.dataRows + 1; // Header row

  expect(screen.getAllByRole('row')).toHaveLength(expectedCount);
}
