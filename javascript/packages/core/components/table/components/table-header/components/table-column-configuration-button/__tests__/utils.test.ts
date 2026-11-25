import { vi } from 'vitest';

import { CellType } from '#core/components/cell/constants';
import { createColumnListChangeHandler } from '../utils';

import type { Mock } from 'vitest';
import type { ControlledTableState } from '#core/components/table/types/table-types';

describe('createColumnListChangeHandler', () => {
  let mockSetColumnOrder: Mock<ControlledTableState['setColumnOrder']>;
  let mockSetColumnVisibility: Mock<ControlledTableState['setColumnVisibility']>;

  beforeEach(() => {
    mockSetColumnOrder = vi.fn<ControlledTableState['setColumnOrder']>();
    mockSetColumnVisibility = vi.fn<ControlledTableState['setColumnVisibility']>();
  });

  it('reorders columns when given position indices', () => {
    const handler = createColumnListChangeHandler(
      [
        { type: CellType.TEXT, id: 'col1', label: 'Column 1', isVisible: true, canHide: false },
        { type: CellType.TEXT, id: 'col2', label: 'Column 2', isVisible: true, canHide: true },
        { type: CellType.TEXT, id: 'col3', label: 'Column 3', isVisible: false, canHide: true },
        { type: CellType.TEXT, id: 'col4', label: 'Column 4', isVisible: true, canHide: true },
      ],
      mockSetColumnOrder,
      mockSetColumnVisibility
    );

    handler({ oldIndex: 0, newIndex: 2 });

    expect(mockSetColumnOrder).toHaveBeenCalledWith(['col1', 'col3', 'col4', 'col2']);
    expect(mockSetColumnVisibility).not.toHaveBeenCalled();
  });

  it('toggles column visibility when newIndex is -1', () => {
    const handler = createColumnListChangeHandler(
      [
        { type: CellType.TEXT, id: 'col1', label: 'Column 1', isVisible: true, canHide: false },
        { type: CellType.TEXT, id: 'col2', label: 'Column 2', isVisible: true, canHide: true },
        { type: CellType.TEXT, id: 'col3', label: 'Column 3', isVisible: false, canHide: true },
        { type: CellType.TEXT, id: 'col4', label: 'Column 4', isVisible: true, canHide: true },
      ],
      mockSetColumnOrder,
      mockSetColumnVisibility
    );

    handler({ oldIndex: 0, newIndex: -1 });

    expect(mockSetColumnOrder).not.toHaveBeenCalled();

    const [updaterFunction] = mockSetColumnVisibility.mock.calls[0];
    if (typeof updaterFunction !== 'function') {
      throw new Error('Expected a column visibility updater function');
    }

    // col2 starts visible (true) and should be toggled to hidden (false)
    const currentVisibility = { col1: true, col2: true, col3: false, col4: true };
    const result = updaterFunction(currentVisibility);

    expect(result).toEqual({
      col1: true,
      col2: false,
      col3: false,
      col4: true,
    });
  });

  it('handles empty columns array', () => {
    const handler = createColumnListChangeHandler([], mockSetColumnOrder, mockSetColumnVisibility);

    handler({ oldIndex: 0, newIndex: 1 });

    // Should call setColumnOrder (arrayMove behavior with empty array may vary)
    expect(mockSetColumnOrder).toHaveBeenCalled();
    expect(mockSetColumnVisibility).not.toHaveBeenCalled();
  });
});
