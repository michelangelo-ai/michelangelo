import { useField } from 'react-final-form';

import { isEmptyFieldValue } from '#core/components/form/utils/is-empty-field-value';

import type { ReactNode } from 'react';
import type { ConditionLayoutConfig } from './types';

/**
 * Renders `children` when `layout` evaluates to true against the current form state.
 */
export function FormCondition({
  layout,
  children,
}: {
  layout: ConditionLayoutConfig;
  children: ReactNode;
}) {
  const { input } = useField(layout.when, { subscription: { value: true } });

  return shouldRender(layout, input.value) ? <>{children}</> : null;
}

function shouldRender(layout: ConditionLayoutConfig, value: unknown): boolean {
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
