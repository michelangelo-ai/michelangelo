import { useField as useReactFinalFormField } from 'react-final-form';

import type { FieldInput, FieldState } from '../types';

export function useField<T = unknown>(name: string): { input: FieldInput<T>; meta: FieldState } {
  const field = useReactFinalFormField<T>(name);

  const input: FieldInput<T> = {
    value: field.input.value,
    name: field.input.name,
    onChange: field.input.onChange,
    onBlur: field.input.onBlur,
  };

  const meta: FieldState = {
    // TODO: field.meta.error is typed as any, do we need better refinement?
    error: field.meta.error as string,
    touched: !!field.meta.touched,
  };

  return { input, meta };
}
