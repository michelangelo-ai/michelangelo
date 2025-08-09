import type { PageSizeOption } from './types';

/**
 * Normalizes the given page size to the nearest valid page size from the configuration.
 * Ensures pagination stays within the allowed set of page sizes rather than accepting arbitrary values.
 *
 * @example
 * ```ts
 * const pageSizes = [{ id: 10, label: '10' }, { id: 25, label: '25' }, { id: 50, label: '50' }];
 *
 * normalizePageSize(15, pageSizes); // returns 25 (next largest)
 * normalizePageSize(5, pageSizes);  // returns 10 (minimum)
 * normalizePageSize(100, pageSizes); // returns 50 (maximum)
 * normalizePageSize(null, pageSizes); // returns 10 (minimum)
 * ```
 */
export function normalizePageSize(
  pageSize: number | null | undefined,
  pageSizes: PageSizeOption[]
): number {
  const minimum = pageSizes[0].id;

  if (pageSize == null || pageSize <= minimum) {
    return minimum;
  }

  const nextLargestPageSize = pageSizes.find((option) => pageSize <= option.id);
  const pageSizeOption = nextLargestPageSize ?? pageSizes[pageSizes.length - 1];
  return pageSizeOption.id;
}
