import { useField } from 'react-final-form';

import { isEmptyFieldValue } from '#core/components/form/utils/is-empty-field-value';
import { useRepeatedLayoutContext } from '#core/providers/repeated-layout-provider/use-repeated-layout-context';
import { buildIndexedFieldId } from './build-indexed-field-id';

import type { ReactNode } from 'react';
import type { ConditionLayoutConfig } from './types';

/**
 * Renders `children` when `layout` evaluates to true against the current form state.
 *
 * `layout.when` is an entity-relative field path (e.g. `items.name`), not a literal
 * field id. Inside a `RepeatedLayoutProvider`, it's resolved through
 * `buildIndexedFieldId` against the current item's index before subscribing, so the
 * same config can be reused across every item in a repeated layout.
 */
export function FormCondition({
  layout,
  children,
}: {
  layout: ConditionLayoutConfig;
  children: ReactNode;
}) {
  const repeatedContext = useRepeatedLayoutContext();

  let fieldId = layout.when;
  if (repeatedContext) {
    fieldId = buildIndexedFieldId({
      rootFieldPath: repeatedContext.rootFieldPath,
      entityId: layout.when,
      index: repeatedContext.index,
    });
  }

  const { input } = useField(fieldId, { subscription: { value: true } });

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
