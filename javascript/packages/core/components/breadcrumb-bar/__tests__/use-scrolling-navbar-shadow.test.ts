import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useScrollingNavbarShadow } from '../use-scrolling-navbar-shadow';

function setScrollY(value: number) {
  Object.defineProperty(window, 'scrollY', { value, configurable: true });
}

describe('useScrollingNavbarShadow', () => {
  beforeEach(() => setScrollY(0));
  afterEach(() => setScrollY(0));

  it('returns isScrolled: false when at the top', () => {
    const { result } = renderHook(() => useScrollingNavbarShadow());
    expect(result.current.isScrolled).toBe(false);
  });

  it('returns isScrolled: true after scrolling down', () => {
    const { result } = renderHook(() => useScrollingNavbarShadow());

    act(() => {
      setScrollY(100);
      window.dispatchEvent(new Event('scroll'));
    });

    expect(result.current.isScrolled).toBe(true);
  });

  it('returns isScrolled: false after scrolling back to top', () => {
    const { result } = renderHook(() => useScrollingNavbarShadow());

    act(() => {
      setScrollY(100);
      window.dispatchEvent(new Event('scroll'));
    });
    act(() => {
      setScrollY(0);
      window.dispatchEvent(new Event('scroll'));
    });

    expect(result.current.isScrolled).toBe(false);
  });

  it('removes the scroll listener on unmount', () => {
    const spy = vi.spyOn(window, 'removeEventListener');
    const { unmount } = renderHook(() => useScrollingNavbarShadow());

    unmount();

    expect(spy).toHaveBeenCalledWith('scroll', expect.any(Function));
    spy.mockRestore();
  });
});
