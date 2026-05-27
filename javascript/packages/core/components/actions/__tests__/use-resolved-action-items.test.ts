import { renderHook } from '@testing-library/react';

import { useResolvedActionItems } from '#core/components/actions/use-resolved-action-items';

import type { ActionConfig } from '#core/components/actions/types';

const customAction = (label: string, disabled?: { condition: boolean; message?: string }[]) =>
  ({
    display: { label },
    modal: { type: 'custom', component: () => null },
    ...(disabled ? { disabled } : {}),
  }) as ActionConfig;

describe('useResolvedActionItems', () => {
  it('returns one item per action with display and onClick wired to onSelect', () => {
    const onSelect = vi.fn();
    const action = customAction('Delete');
    const { result } = renderHook(() => useResolvedActionItems([action], onSelect));

    expect(result.current).toHaveLength(1);
    expect(result.current[0].display).toEqual({ label: 'Delete' });
    expect(result.current[0].disabled).toBe(false);
    expect(result.current[0].disabledMessage).toBeUndefined();

    result.current[0].onClick();
    expect(onSelect).toHaveBeenCalledWith(action);
  });

  it('extracts the first matching disabled rule into the resolved item', () => {
    const onSelect = vi.fn();
    const action = customAction('Edit', [
      { condition: false, message: 'Should not appear' },
      { condition: true, message: 'Read-only' },
      { condition: true, message: 'Should not appear either' },
    ]);
    const { result } = renderHook(() => useResolvedActionItems([action], onSelect));

    expect(result.current[0].disabled).toBe(true);
    expect(result.current[0].disabledMessage).toBe('Read-only');
  });

  it('each item resolves onClick to its own action', () => {
    const onSelect = vi.fn();
    const a = customAction('A');
    const b = customAction('B');
    const { result } = renderHook(() => useResolvedActionItems([a, b], onSelect));

    result.current[1].onClick();
    expect(onSelect).toHaveBeenCalledWith(b);
  });
});
