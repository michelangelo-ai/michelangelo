import { useField } from 'react-final-form';

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
  return value === layout.is;
}
