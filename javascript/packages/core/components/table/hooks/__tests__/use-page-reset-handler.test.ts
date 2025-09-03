import { renderHook } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { usePageResetHandler } from '../use-page-reset-handler';

describe('usePageResetHandler', () => {
  it('does not reset when page is valid', () => {
    const mockGotoPage = vi.fn();

    const { result } = renderHook(() =>
      usePageResetHandler({
        gotoPage: mockGotoPage,
        pageCount: 5,
        paginationState: { pageIndex: 2, pageSize: 10 }, // Page 3 of 5 = valid
      })
    );

    expect(result.current.isResetting).toBe(false);
    expect(mockGotoPage).not.toHaveBeenCalled();
  });

  it('does not reset when already on page 1', () => {
    const mockGotoPage = vi.fn();

    const { result } = renderHook(() =>
      usePageResetHandler({
        gotoPage: mockGotoPage,
        pageCount: 1,
        paginationState: { pageIndex: 0, pageSize: 10 }, // Page 1 of 1 = valid
      })
    );

    expect(result.current.isResetting).toBe(false);
    expect(mockGotoPage).not.toHaveBeenCalled();
  });

  it('resets page when current page becomes invalid', () => {
    const mockGotoPage = vi.fn();

    const { result, rerender } = renderHook(
      ({ pageCount }) =>
        usePageResetHandler({
          gotoPage: mockGotoPage,
          pageCount,
          paginationState: { pageIndex: 4, pageSize: 10 }, // Page 5
        }),
      {
        initialProps: { pageCount: 5 }, // Page 5 of 5 = valid
      }
    );

    expect(result.current.isResetting).toBe(false);
    expect(mockGotoPage).not.toHaveBeenCalled();

    // Change to invalid page scenario (filtering reduced pages)
    rerender({ pageCount: 2 }); // Page 5 of 2 pages = invalid
    expect(mockGotoPage).toHaveBeenCalledWith(0);
    expect(result.current.isResetting).toBe(true);
  });

  it('clears resetting flag when page reset completes', () => {
    const mockGotoPage = vi.fn();

    const { result, rerender } = renderHook(
      ({ pageIndex, pageCount }) =>
        usePageResetHandler({
          gotoPage: mockGotoPage,
          pageCount,
          paginationState: { pageIndex, pageSize: 10 },
        }),
      {
        initialProps: { pageIndex: 4, pageCount: 2 }, // Page 5 of 2 = invalid
      }
    );

    // Simulate successful page reset (pageIndex becomes 0)
    expect(result.current.isResetting).toBe(true);
    expect(mockGotoPage).toHaveBeenCalledWith(0);

    // Page index becomes 0, so should clear resetting flag
    rerender({ pageIndex: 0, pageCount: 2 });
    expect(result.current.isResetting).toBe(false);
  });
});
