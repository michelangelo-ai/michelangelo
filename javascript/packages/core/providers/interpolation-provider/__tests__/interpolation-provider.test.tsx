import { renderHook } from '@testing-library/react';

import { InterpolationProvider } from '../interpolation-provider';
import { useInterpolationContext } from '../use-interpolation-context';

describe('InterpolationProvider', () => {
  test('provides interpolation context to children', () => {
    const value = { user: { name: 'Test User' }, project: { id: 'test-project' } };

    const { result } = renderHook(() => useInterpolationContext(), {
      wrapper: ({ children }) => (
        <InterpolationProvider value={value}>{children}</InterpolationProvider>
      ),
    });

    expect(result.current).toEqual(value);
  });

  test('returns empty object when no provider', () => {
    const { result } = renderHook(() => useInterpolationContext());

    expect(result.current).toEqual({});
  });

  test('returns empty object when provider has no value', () => {
    const { result } = renderHook(() => useInterpolationContext(), {
      wrapper: ({ children }) => <InterpolationProvider>{children}</InterpolationProvider>,
    });

    expect(result.current).toEqual({});
  });
});
