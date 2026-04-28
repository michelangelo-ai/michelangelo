import { renderHook } from '@testing-library/react';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { useCellStyles } from '../hooks';

import type { Theme } from 'baseui';

describe('useCellStyles', () => {
  it('should return an empty object if style is undefined', () => {
    const { result } = renderHook(
      () => useCellStyles({ record: {}, style: undefined }),
      buildWrapper([getBaseProviderWrapper()])
    );
    expect(result.current).toEqual({});
  });

  it('should return the style object if style is not a function', () => {
    const style = { color: 'red' };
    const { result } = renderHook(
      () => useCellStyles({ record: {}, style }),
      buildWrapper([getBaseProviderWrapper()])
    );
    expect(result.current).toEqual(style);
  });

  it('should call the style function with record and theme and return the result', () => {
    const record = { id: 1 };
    // Theme is defined in rtl-wrappers.tsx, so theme.colors.contentPositive can change
    // during baseui updates.
    const style = ({ theme }: { theme: Theme }) => {
      return { backgroundColor: theme.colors.contentPositive };
    };
    const { result } = renderHook(
      () => useCellStyles({ record, style }),
      buildWrapper([getBaseProviderWrapper()])
    );
    expect(result.current).toEqual({ backgroundColor: '#0E8345' });
  });
});
