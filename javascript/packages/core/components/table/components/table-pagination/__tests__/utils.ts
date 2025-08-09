import { normalizePageSize } from '../utils';

import type { PageSizeOption } from '../types';

describe('table pagination utils', () => {
  describe('normalizePageSize', () => {
    const pageSizes: PageSizeOption[] = [
      { id: 10, label: '10' },
      { id: 25, label: '25' },
      { id: 50, label: '50' },
    ];

    test.each([
      // Null/undefined cases
      { input: null, expected: 10, description: 'null input returns minimum' },
      { input: undefined, expected: 10, description: 'undefined input returns minimum' },

      // Below minimum cases
      { input: 5, expected: 10, description: 'below minimum returns minimum' },
      { input: 0, expected: 10, description: 'zero returns minimum' },
      { input: -1, expected: 10, description: 'negative returns minimum' },

      // Exact matches
      { input: 10, expected: 10, description: 'exact match returns same value' },
      { input: 25, expected: 25, description: 'exact match returns same value' },
      { input: 50, expected: 50, description: 'exact match returns same value' },

      // Between values (next largest)
      { input: 15, expected: 25, description: 'between values returns next largest' },
      { input: 20, expected: 25, description: 'between values returns next largest' },
      { input: 30, expected: 50, description: 'between values returns next largest' },
      { input: 45, expected: 50, description: 'between values returns next largest' },

      // Above maximum
      { input: 100, expected: 50, description: 'above maximum returns maximum' },
      { input: 1000, expected: 50, description: 'above maximum returns maximum' },
    ])('$description', ({ input, expected }) => {
      expect(normalizePageSize(input, pageSizes)).toBe(expected);
    });

    test('works with single page size option', () => {
      const singleOption = [{ id: 20, label: '20' }];

      expect(normalizePageSize(10, singleOption)).toBe(20);
      expect(normalizePageSize(30, singleOption)).toBe(20);
    });
  });
});
