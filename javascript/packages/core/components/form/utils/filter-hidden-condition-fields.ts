import { getIn, setIn } from 'final-form';

import type { FormConfig, LayoutItem } from '#core/components/form/types/config-types';

/**
 * Strips values for fields nested under a `condition` layout that currently evaluates to
 * hidden, so stale data entered while the condition was true never gets submitted after
 * the user navigates away from it.
 */
export function filterHiddenConditionFields<T extends Record<string, unknown>>(
  values: T,
  config: FormConfig<T>
): T {
  return stripHiddenConditions(config.layout, values);
}

function stripHiddenConditions<T extends Record<string, unknown>>(
  layout: LayoutItem[],
  values: T
): T {
  return layout.reduce((acc, item) => {
    if (typeof item === 'string') return acc;

    if (item.type === 'condition' && getIn(acc, item.when) !== item.is) {
      return stripFieldNames(item.items, acc);
    }

    return stripHiddenConditions(item.items, acc);
  }, values);
}

function stripFieldNames<T extends Record<string, unknown>>(layout: LayoutItem[], values: T): T {
  return layout.reduce((acc, item) => {
    if (typeof item === 'string') {
      // cast: setIn's return type is `object`; it always returns a value of the same shape as `acc`
      return (setIn(acc, item, undefined) ?? {}) as T;
    }

    return stripFieldNames(item.items, acc);
  }, values);
}
