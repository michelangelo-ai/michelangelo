import { DeepPartial } from '#core/types/utility-types';
import { getResponsiveColumnWidth } from '../get-responsive-column-width';

import type { Theme } from 'baseui';

describe('getResponsiveColumnWidth', () => {
  it('creates responsive max width styles for multiple breakpoints', () => {
    const mockTheme: Pick<Theme, 'breakpoints'> = {
      breakpoints: {
        small: 320,
        medium: 600,
        large: 1280,
      },
    };

    const result = getResponsiveColumnWidth(mockTheme as Theme);

    expect(result).toEqual({
      '@media screen and (min-width: 320px)': { maxWidth: '150px' },
      '@media screen and (min-width: 600px)': { maxWidth: '300px' },
      '@media screen and (min-width: 1280px)': { maxWidth: '450px' },
    });
  });

  it('returns empty object for theme with no breakpoints', () => {
    const mockTheme: Pick<Theme, 'breakpoints'> = {
      // @ts-expect-error intentional empty breakpoints object
      breakpoints: {},
    };

    const result = getResponsiveColumnWidth(mockTheme as Theme);

    expect(result).toEqual({});
  });

  it('handles single breakpoint correctly', () => {
    const mockTheme: DeepPartial<Pick<Theme, 'breakpoints'>> = {
      breakpoints: {
        small: 480,
      },
    };

    const result = getResponsiveColumnWidth(mockTheme as Theme);

    expect(result).toEqual({
      '@media screen and (min-width: 480px)': { maxWidth: '150px' },
    });
  });

  it('preserves breakpoint order from theme object', () => {
    const mockTheme: Pick<Theme, 'breakpoints'> = {
      breakpoints: {
        large: 1200,
        small: 320,
        medium: 768,
      },
    };

    const result = getResponsiveColumnWidth(mockTheme as Theme);
    const keys = Object.keys(result);

    expect(keys[0]).toBe('@media screen and (min-width: 1200px)');
    expect(keys[1]).toBe('@media screen and (min-width: 320px)');
    expect(keys[2]).toBe('@media screen and (min-width: 768px)');
  });
});
