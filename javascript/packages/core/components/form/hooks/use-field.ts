import { useField as useReactFinalFormField } from 'react-final-form';

import { combineValidators } from '#core/components/form/validation/combine-validators';
import { required as requiredValidator } from '#core/components/form/validation/validators';

import type { FieldValidator } from '#core/components/form/validation/types';
import type { FieldInput, FieldState } from '../types';

export function useField<T = unknown>(
  name: string,
  options?: { validate?: FieldValidator; required?: boolean; defaultValue?: T; initialValue?: T }
): { input: FieldInput<T>; meta: FieldState } {
  const composedValidate = options?.required
    ? combineValidators(requiredValidator(), ...(options.validate ? [options.validate] : []))
    : options?.validate;

  const validate = composedValidate ? (value: T) => composedValidate(value as unknown) : undefined;

  const field = useReactFinalFormField<T>(name, {
    validate,
    defaultValue: options?.defaultValue,
    initialValue: options?.initialValue,
  });

  const input: FieldInput<T> = {
    value: field.input.value,
    name: field.input.name,
    onChange: field.input.onChange,
    onBlur: field.input.onBlur,
  };

  const meta: FieldState = {
    error: typeof field.meta.error === 'string' ? field.meta.error : undefined,
    touched: !!field.meta.touched,
  };

  return { input, meta };
}
