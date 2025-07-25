import { vi } from 'vitest';

import type { Header } from '@tanstack/react-table';
import type { ReactNode } from 'react';
import type { TableData } from '#core/components/table/types/data-types';

export const getTanstackHeaderFixture = (overrides?: {
  id?: string;
  content?: ReactNode;
}): Header<TableData, unknown> =>
  ({
    id: overrides?.id ?? 'test-header',
    column: { columnDef: { header: overrides?.content ?? 'Test Header' } },
    getContext: vi.fn(() => ({})),
  }) as unknown as Header<TableData, unknown>;
