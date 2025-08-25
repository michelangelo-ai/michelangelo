import type { StickyColumnConfigs } from './types';

// Measured width of the table selection column (checkbox + padding)
// This value was determined by measuring the actual rendered selection column
// If the selection column styling changes, this may need to be updated
const SELECTION_COLUMN_WIDTH = 56;

/**
 * Generates sticky column configurations for table rows.
 *
 * Determines which columns should be sticky and their positioning based on:
 * - Whether row selection (checkbox column) is enabled
 * - The index of the last column for right-side sticking
 *
 * When row selection is enabled:
 * - Column 0 (checkbox) sticks to left at position 0 with no shadow
 * - Column 1 (first data) sticks to left at position 56px with right shadow
 * - Last column sticks to right with left shadow
 *
 * When row selection is disabled:
 * - Column 1 (first data) sticks to left at position 0 with right shadow
 * - Last column sticks to right with left shadow
 *
 * @param enableRowSelection - Whether the table has a selection checkbox column
 * @param lastColumnIndex - Zero-based index of the rightmost column
 * @returns Configuration object mapping column indices to sticky settings
 */
export function getTableStickyConfigs(
  enableRowSelection: boolean,
  lastColumnIndex: number
): StickyColumnConfigs {
  const configs: StickyColumnConfigs = {};

  if (enableRowSelection) {
    configs[0] = {
      stickySide: 'left',
      position: 0,
      shadowSide: 'none',
    };

    configs[1] = {
      stickySide: 'left',
      position: SELECTION_COLUMN_WIDTH,
      shadowSide: 'right',
    };
  } else {
    configs[1] = {
      stickySide: 'left',
      position: 0,
      shadowSide: 'right',
    };
  }

  configs[lastColumnIndex] = {
    stickySide: 'right',
    position: 0,
    shadowSide: 'left',
  };

  return configs;
}
