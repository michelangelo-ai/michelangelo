import { merge } from 'lodash';

import { CellType } from '#core/components/cell/constants';

import type { DeepPartial } from '#core/types/utility-types';
import type { TableConfig } from '../types';

/**
 * Factory for creating TableConfig test fixtures.
 *
 * @param base - Partial object shared across all test fixtures for a test suite
 * @returns Function that generates a complete table config using overrides.
 *
 * @example
 * ```typescript
 * // Setup base configuration for test suite
 * const buildConfig = buildTableConfigFactory();
 * const basicConfig = buildConfig();
 *
 * // Custom variations
 * const searchDisabledConfig = buildConfig({
 *   disableSearch: true,
 *   emptyState: { title: 'Custom Empty State' }
 * });
 * ```
 */
export const buildTableConfigFactory = <T extends object = object>(
  base: DeepPartial<TableConfig<T>> = {}
) => {
  return (overrides: DeepPartial<TableConfig<T>> = {}): TableConfig<T> => {
    const required: TableConfig<T> = {
      columns: [
        { id: 'name', label: 'Name', type: CellType.TEXT },
        { id: 'status', label: 'Status', type: CellType.TEXT },
        { id: 'createdAt', label: 'Created', type: CellType.DATE },
      ],
      emptyState: {
        title: 'No items',
        content: 'No items found.',
      },
      disablePagination: false,
      disableSorting: false,
      disableSearch: false,
      disableFilters: false,
      pageSizes: [
        { id: 10, label: '10' },
        { id: 25, label: '25' },
        { id: 50, label: '50' },
      ],
      enableStickySides: true,
    };

    return merge({}, required, base, overrides);
  };
};
