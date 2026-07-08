import { useMemo } from 'react';

import type { ActionConfig, Data, ResolvedActionItem } from './types';

/** Memoized on actions/onSelect so returned items have stable identity for reference comparisons (e.g. hover tracking). */
export function useResolvedActionItems<T extends Data>(
  actions: ActionConfig<T>[],
  onSelect: (action: ActionConfig<T>) => void
): ResolvedActionItem[] {
  return useMemo(
    () =>
      actions.map((action) => {
        const matchingRule = action.disabled?.find((rule) => rule.condition);
        return {
          display: action.display,
          hierarchy: action.hierarchy,
          disabled: !!matchingRule,
          disabledMessage: matchingRule?.message,
          onClick: () => onSelect(action),
        };
      }),
    [actions, onSelect]
  );
}
