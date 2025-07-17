import { renderHook } from '@testing-library/react';

import { RepeatedLayoutProvider } from '../repeated-layout-provider';
import { useRepeatedLayoutContext } from '../use-repeated-layout-context';

describe('RepeatedLayoutProvider', () => {
  test('provides repeated layout context to children', () => {
    const state = { index: 2, rootFieldPath: 'items.data' };

    const { result } = renderHook(() => useRepeatedLayoutContext(), {
      wrapper: ({ children }) => (
        <RepeatedLayoutProvider {...state}>{children}</RepeatedLayoutProvider>
      ),
    });

    expect(result.current).toEqual(state);
  });

  test('returns undefined when no provider', () => {
    const { result } = renderHook(() => useRepeatedLayoutContext());

    expect(result.current).toBeUndefined();
  });
});
