import { act, renderHook } from '@testing-library/react';

import { useScrollRatio } from '../use-scroll-ratio';

describe('useScrollRatio', () => {
  it('should return -1 initially', () => {
    const { result } = renderHook(() => useScrollRatio([]));

    expect(result.current.scrollRatio).toBe(-1);
    expect(result.current.tableRef.current).toBe(null);
    expect(typeof result.current.handleScrollRatioUpdate).toBe('function');
  });

  it('should calculate scroll ratio correctly', () => {
    const mockElement = {
      scrollWidth: 200,
      clientWidth: 100,
      scrollLeft: 50,
    } as unknown as HTMLElement;

    const { result } = renderHook(() => useScrollRatio([]));
    result.current.tableRef.current = mockElement;

    act(() => {
      result.current.handleScrollRatioUpdate();
    });

    expect(result.current.scrollRatio).toBe(0.5);
  });

  it('should handle full scroll', () => {
    const mockElement = {
      scrollWidth: 200,
      clientWidth: 100,
      scrollLeft: 100,
    } as unknown as HTMLElement;

    const { result } = renderHook(() => useScrollRatio([]));
    result.current.tableRef.current = mockElement;

    act(() => {
      result.current.handleScrollRatioUpdate();
    });

    expect(result.current.scrollRatio).toBe(1);
  });

  it('should return -1 when no scroll is needed', () => {
    const mockElement = {
      scrollWidth: 100,
      clientWidth: 100,
      scrollLeft: 0,
    } as unknown as HTMLElement;

    const { result } = renderHook(() => useScrollRatio([]));
    result.current.tableRef.current = mockElement;

    act(() => {
      result.current.handleScrollRatioUpdate();
    });

    expect(result.current.scrollRatio).toBe(-1);
  });

  it('should handle partial scroll positions', () => {
    const mockElement = {
      scrollWidth: 300,
      clientWidth: 100,
      scrollLeft: 50,
    } as unknown as HTMLElement;

    const { result } = renderHook(() => useScrollRatio([]));
    result.current.tableRef.current = mockElement;

    act(() => {
      result.current.handleScrollRatioUpdate();
    });

    expect(result.current.scrollRatio).toBe(0.25);
  });

  it('should handle rounding correctly', () => {
    const mockElement = {
      scrollWidth: 200,
      clientWidth: 100,
      scrollLeft: 33.7,
    } as unknown as HTMLElement;

    const { result } = renderHook(() => useScrollRatio([]));
    result.current.tableRef.current = mockElement;

    act(() => {
      result.current.handleScrollRatioUpdate();
    });

    expect(result.current.scrollRatio).toBe(0.34);
  });
});
