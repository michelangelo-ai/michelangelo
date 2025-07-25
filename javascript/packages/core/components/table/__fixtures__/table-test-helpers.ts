import { fromPairs } from 'lodash';

import type { TableColumn } from '../types/column-types';

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

export function buildTableColumns(numberOfColumns: number): TableColumn[] {
  return getArrayWithLength(numberOfColumns).map((columnId) => ({
    id: `col${columnId}`,
    label: `Column${columnId}`,
    accessor: `col${columnId}`,
  }));
}

export function getArrayWithLength(length: number): number[] {
  return Array.from({ length }, (_, i) => i + 1);
}
