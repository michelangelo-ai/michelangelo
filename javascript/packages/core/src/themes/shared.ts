import type { Grid } from 'baseui/themes';

/**
 * @description
 * Overrides the default BaseWeb theme's `grid` settings to customize
 * the application's layout constraints for wider screens.
 *
 * The `grid` property contains the following overrides:
 * - `margins`: An array defining left and right margins at different BaseWeb theme breakpoints:
 * - Index 0: Applied to viewports >= the 'small' breakpoint.
 * - Index 1: Applied to viewports >= the 'medium' breakpoint.
 * - Index 2: Applied to viewports >= the 'large' breakpoint.
 * - `maxWidth`: The maximum width (in pixels) for the application's content area within the grid.
 * Calculated as 1920px (intended max width) minus 36px left margin and 36px right margin.
 *
 * @remarks
 * A wider application makes sense for Michelangelo's dense interface. Most developers have
 * wide screens, so they will benefit from a wider application.
 */
export const GRID_OVERRIDES: { grid: Partial<Grid> } = {
  grid: {
    margins: [16, 36, 36],
    maxWidth: 1848, // 1920 - 36 - 36
  },
};
