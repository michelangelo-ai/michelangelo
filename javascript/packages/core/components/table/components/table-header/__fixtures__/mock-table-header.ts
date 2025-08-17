import { vi } from 'vitest';

import type { Header } from '@tanstack/react-table';
import type { ReactNode } from 'react';
import type { TableData } from '#core/components/table/types/data-types';

export const getTanstackHeaderFixture = (overrides?: {
  id?: string;
  content?: ReactNode;
  canSort?: boolean;
  sortDirection?: false | 'asc' | 'desc';
}): Header<TableData, unknown> =>
  ({
    id: overrides?.id ?? 'test-header',
    column: {
      columnDef: { header: overrides?.content ?? 'Test Header' },
      getCanSort: vi.fn(() => overrides?.canSort ?? true),
      getToggleSortingHandler: vi.fn(() => vi.fn()),
      getIsSorted: vi.fn(() => overrides?.sortDirection ?? false),
    },
    getContext: vi.fn(() => ({})),
  }) as unknown as Header<TableData, unknown>;
