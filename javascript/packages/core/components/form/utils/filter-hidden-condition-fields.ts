import { get } from 'lodash';

import { deleteByPath } from '#core/components/form/utils/delete-by-path';

import type { FormConfig, LayoutItem } from '#core/components/form/types/config-types';

/**
 * Strips values for fields nested under an `is` condition that currently evaluates to
 * hidden, so stale data entered while the condition was true never gets submitted after
 * the user navigates away from it.
 *
 * Only the `is` operator is handled for now — conditions using `isNot`/`isEmpty`/
 * `containsAny` are left untouched (their fields still render conditionally via
 * `FormCondition`, but their values aren't yet stripped from the submitted payload).
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

    if (item.type === 'condition' && 'is' in item && get(acc, item.when) !== item.is) {
      return stripFieldNames(item.items, acc);
    }

    return stripHiddenConditions(item.items, acc);
  }, values);
}

function stripFieldNames<T extends Record<string, unknown>>(layout: LayoutItem[], values: T): T {
  return layout.reduce((acc, item) => {
    if (typeof item === 'string') {
      return deleteByPath(acc, item);
    }

    return stripFieldNames(item.items, acc);
  }, values);
}
