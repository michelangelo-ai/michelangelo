import { getIn, setIn } from 'final-form';

import { evaluateCondition } from '#core/components/form/layout/condition/evaluate-condition';

import type { FormConfig, LayoutItem } from '#core/components/form/types/config-types';

/**
 * Strips values for fields nested under a condition that currently evaluates to hidden,
 * so stale data entered while the condition was true never gets submitted after the user
 * navigates away from it.
 *
 * Uses the same `evaluateCondition` logic as `FormCondition`, so every operator
 * (`is`/`isNot`/`isEmpty`/`containsAny`) is stripped consistently with what's rendered.
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

    if (item.type === 'condition' && !evaluateCondition(item, getIn(acc, item.when))) {
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
