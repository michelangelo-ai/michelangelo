import { act, renderHook } from '@testing-library/react';
import { vi } from 'vitest';

import { useTopBarLinks } from '../use-top-bar-links';

describe('useTopBarLinks', () => {
  let openSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    openSpy = vi.spyOn(window, 'open').mockImplementation(() => null);
  });

  afterEach(() => {
    openSpy.mockRestore();
  });

  it('returns Docs and Help as main items', () => {
    const { result } = renderHook(() => useTopBarLinks());

    expect(result.current.mainItems).toEqual([{ label: 'Docs' }, { label: 'Help' }]);
  });

  it('opens the default docs URL when no override is provided', () => {
    const { result } = renderHook(() => useTopBarLinks());

    act(() => {
      result.current.handleMainItemSelect({ label: 'Docs' });
    });

    expect(openSpy).toHaveBeenCalledWith(
      'https://michelangelo-ai.org/docs/',
      '_blank',
      'noopener,noreferrer'
    );
  });

  it('opens the default help URL when no override is provided', () => {
    const { result } = renderHook(() => useTopBarLinks());

    act(() => {
      result.current.handleMainItemSelect({ label: 'Help' });
    });

    expect(openSpy).toHaveBeenCalledWith(
      'https://michelangelo-ai.org/docs/#support--community',
      '_blank',
      'noopener,noreferrer'
    );
  });

  it('opens an adopter-provided URL instead of the default', () => {
    const { result } = renderHook(() =>
      useTopBarLinks({ docsUrl: 'https://adopter.example.com/docs' })
    );

    act(() => {
      result.current.handleMainItemSelect({ label: 'Docs' });
    });

    expect(openSpy).toHaveBeenCalledWith(
      'https://adopter.example.com/docs',
      '_blank',
      'noopener,noreferrer'
    );
  });
});
