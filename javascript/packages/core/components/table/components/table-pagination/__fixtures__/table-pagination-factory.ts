import { merge } from 'lodash';

import type { DeepPartial } from '#core/types/utility-types';
import type { TablePaginationProps } from '../types';

/**
 * Factory for creating TablePagination test fixtures.
 * Provides minimal required properties for rendering with sensible defaults.
 *
 * @param base - Partial object shared across all test fixtures for a test suite
 * @returns Function that generates complete pagination props using overrides.
 *
 * @example
 * ```typescript
 * // Setup base configuration for test suite
 * const buildPaginationProps = buildTablePaginationPropsFactory({
 *   gotoPage: mockGoToPage,
 *   setPageSize: mockSetPageSize,
 * });
 *
 * const basicProps = buildPaginationProps();
 *
 * // Custom variations
 * const lastPageProps = buildPaginationProps({
 *   state: { pageIndex: 4, pageSize: 10 }
 * });
 *
 * const serverPaginationProps = buildPaginationProps({
 *   fetchPlugin: {
 *     fetchNextPage: mockFetchNextPage,
 *     isFetchNextPageInProgress: true,
 *   }
 * });
 * ```
 */
export const buildTablePaginationPropsFactory = (base: DeepPartial<TablePaginationProps> = {}) => {
  return (overrides: DeepPartial<TablePaginationProps> = {}): TablePaginationProps => {
    const required: TablePaginationProps = {
      pageSizes: [
        { id: 10, label: '10' },
        { id: 20, label: '20' },
        { id: 50, label: '50' },
      ],
      state: { pageIndex: 0, pageSize: 10 },
      pageCount: 5,
      gotoPage: () => {
        // Empty function for testing
      },
      setPageSize: () => {
        // Empty function for testing
      },
    };

    return merge({}, required, base, overrides);
  };
};
