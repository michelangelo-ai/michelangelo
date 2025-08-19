import { vi } from 'vitest';

import { CellType } from '#core/components/cell/constants';

import type { Column } from '@tanstack/react-table';
import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { TableData } from '#core/components/table/types/data-types';

export const getTanstackColumn = (overrides?: {
  canSort?: boolean;
  sortDirection?: false | 'asc' | 'desc';
  canFilter?: boolean;
  filterValue?: string;
  columnConfig?: Partial<ColumnConfig<TableData>>;
}): Column<TableData, unknown> =>
  ({
    id: overrides?.columnConfig?.id ?? 'test-header',
    columnDef: {
      meta: {
        id: overrides?.columnConfig?.id ?? 'test-header',
        label: overrides?.columnConfig?.label,
        type: overrides?.columnConfig?.type ?? CellType.TEXT,
      },
    },
    getCanSort: vi.fn(() => overrides?.canSort ?? true),
    getToggleSortingHandler: vi.fn(() => vi.fn()),
    getIsSorted: vi.fn(() => overrides?.sortDirection ?? false),
    getCanFilter: vi.fn(() => overrides?.canFilter ?? true),
    getFilterValue: vi.fn(() => overrides?.filterValue ?? ''),
    setFilterValue: vi.fn(),
    getContext: vi.fn(() => ({})),
  }) as unknown as Column<TableData, unknown>;
