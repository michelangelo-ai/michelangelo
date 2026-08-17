import { isEmptyFieldValue } from '#core/components/form/utils/is-empty-field-value';

import type { ConditionLayoutConfig } from './types';

/** Evaluates whether a condition layout's `items` should be visible for the given field value. */
export function evaluateCondition(layout: ConditionLayoutConfig, value: unknown): boolean {
  if ('is' in layout) {
    return value === layout.is;
  }

  if ('isNot' in layout) {
    return !isEmptyFieldValue(value) && value !== layout.isNot;
  }

  if ('isEmpty' in layout) {
    return layout.isEmpty ? isEmptyFieldValue(value) : !isEmptyFieldValue(value);
  }

  if ('containsAny' in layout) {
    return Array.isArray(value)
      ? layout.containsAny.some((item) => value.includes(item))
      : layout.containsAny.some((item) => value === item);
  }

  return true;
}
