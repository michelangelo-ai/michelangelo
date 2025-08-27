import { vi } from 'vitest';

import type { Cell, Row } from '@tanstack/react-table';
import type { ReactNode } from 'react';
import type { TableData } from '#core/components/table/types/data-types';

export const getTanstackRowFixture = (overrides?: {
  id?: string;
  cellContents?: ReactNode[];
}): Row<TableData> => {
  const id = overrides?.id ?? 'test-row';
  const cellContents = overrides?.cellContents ?? ['Cell 1', 'Cell 2'];

  return {
    id,
    getVisibleCells: vi.fn(
      () =>
        cellContents.map((content, index) => ({
          id: `${id}-cell-${index}`,
          column: {
            columnDef: {
              cell: content,
            },
          },
          getContext: vi.fn(() => ({})),
        })) as unknown as Cell<TableData, unknown>[]
    ),
    getCanSelect: vi.fn(() => true),
    getIsSelected: vi.fn(() => false),
    toggleSelected: vi.fn(),
    getCanExpand: vi.fn(() => true),
    getIsExpanded: vi.fn(() => false),
    onToggleExpanded: vi.fn(),
  } as unknown as Row<TableData>;
};
