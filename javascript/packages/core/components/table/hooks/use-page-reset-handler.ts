import { useEffect, useState } from 'react';

import type { PaginationState } from '../types/table-types';

/**
 * Resets pagination to first page when current page becomes invalid due to filtering.
 */
export function usePageResetHandler(args: {
  gotoPage: (page: number) => void;
  pageCount: number;
  paginationState: PaginationState;
}) {
  const { paginationState, pageCount, gotoPage } = args;
  const { pageIndex } = paginationState;
  const [isActiveResetting, setIsActiveResetting] = useState(false);

  useEffect(() => {
    const outOfBounds = pageIndex + 1 > pageCount;
    if (pageIndex !== 0 && outOfBounds) {
      setIsActiveResetting(true);
      gotoPage(0);
    }
  }, [pageIndex, pageCount, gotoPage]);

  useEffect(() => {
    if (isActiveResetting && pageIndex === 0) {
      setIsActiveResetting(false);
    }
  }, [isActiveResetting, pageIndex]);

  // Consider both "actively resetting" and "needs reset" as resetting state
  // to avoid empty state flash before useEffect triggers
  const needsReset = pageIndex + 1 > pageCount && pageIndex !== 0;
  return { isResetting: isActiveResetting || needsReset };
}
