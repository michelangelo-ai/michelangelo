import type { Theme } from 'baseui';

const BASE_MAX_WIDTH = 150;

/**
 * Creates responsive max width constraints for table columns based on viewport breakpoints.
 * Multiplies base width by breakpoint index to provide more space on larger screens.
 *
 * @param theme - BaseUI theme containing breakpoint configuration
 * @returns Object with media query keys and corresponding maxWidth values
 *
 * @example
 * // For breakpoints: { small: 320, medium: 600, large: 1280 }
 * // Returns: {
 * //   '@media screen and (min-width: 320px)': { maxWidth: '150px' },
 * //   '@media screen and (min-width: 600px)': { maxWidth: '300px' },
 * //   '@media screen and (min-width: 1280px)': { maxWidth: '450px' }
 * // }
 */
export function getResponsiveColumnWidth(theme: Theme) {
  const result = {};
  const breakpoints = Object.entries(theme.breakpoints);

  for (const [index, [, value]] of breakpoints.entries()) {
    const query = `@media screen and (min-width: ${value}px)`;
    result[query] = { maxWidth: `${BASE_MAX_WIDTH * (index + 1)}px` };
  }

  return result;
}
